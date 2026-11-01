// Copyright (C) 2026 ResonChat contributors.
// SPDX-License-Identifier: AGPL-3.0-or-later
//
// resonbot joins one or more MUCs and optionally translates / anti-spams.
// Reconnect / Stream Management follow Conversations.
//
//	go run -mod=mod ./cmd -config config.toml
package main

import (
	"context"
	"crypto/tls"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/resonchat-org/resonbot/internal/antispam"
	"github.com/resonchat-org/resonbot/internal/bot"
	"github.com/resonchat-org/resonbot/internal/config"
	"github.com/resonchat-org/resonbot/internal/peertrans"
	"github.com/resonchat-org/resonbot/internal/reconnect"
	"github.com/resonchat-org/resonbot/internal/translate"
	"mellium.im/sasl"
	"mellium.im/xmlstream"
	"mellium.im/xmpp"
	"mellium.im/xmpp/dial"
	"mellium.im/xmpp/jid"
	"mellium.im/xmpp/muc"
	"mellium.im/xmpp/mux"
	"mellium.im/xmpp/ping"
	"mellium.im/xmpp/sm"
	"mellium.im/xmpp/stanza"
	"mellium.im/xmpp/stream"
)

type logWriter struct {
	logger *log.Logger
}

func (w logWriter) Write(p []byte) (int, error) {
	w.logger.Printf("%s", p)
	return len(p), nil
}

func main() {
	logger := log.New(os.Stderr, "", log.LstdFlags)
	debug := log.New(io.Discard, "DEBUG ", log.LstdFlags)

	configPath := "config.toml"
	flags := flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	flags.StringVar(&configPath, "config", configPath, "path to TOML config file")
	flags.Usage = func() {
		fmt.Fprintf(flags.Output(), `Usage: %s -config config.toml

All settings come from TOML (see config.example.toml). Enable features with
translate.enabled / antispam.enabled / retract.enabled.

`, flags.Name())
		flags.PrintDefaults()
	}
	if err := flags.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		logger.Fatal(err)
	}
	if err := config.MustExist(configPath); err != nil {
		logger.Fatalf("config %s: %v", configPath, err)
	}
	file, err := config.Load(configPath)
	if err != nil {
		logger.Fatalf("config: %v", err)
	}

	if file.Log.Verbose || file.Log.XML {
		debug.SetOutput(os.Stderr)
	}
	var xmlIn, xmlOut io.Writer
	if file.Log.XML {
		xmlIn = logWriter{log.New(os.Stdout, "IN ", log.LstdFlags)}
		xmlOut = logWriter{log.New(os.Stdout, "OUT ", log.LstdFlags)}
	}

	parsedAddr, err := jid.Parse(file.XMPP.JID)
	if err != nil {
		logger.Fatalf("xmpp.jid: %v", err)
	}

	rooms := make([]bot.Room, 0, len(file.Rooms))
	for i, rc := range file.Rooms {
		bare, err := jid.Parse(rc.JID)
		if err != nil {
			logger.Fatalf("rooms[%d].jid: %v", i, err)
		}
		bare = bare.Bare()
		full, err := bare.WithResource(rc.Nick)
		if err != nil {
			logger.Fatalf("rooms[%d].nick: %v", i, err)
		}
		rooms = append(rooms, bot.Room{
			Bare:     bare,
			Full:     full,
			Nick:     rc.Nick,
			Password: rc.Password,
		})
	}

	opt := bot.Options{Rooms: rooms, Retract: file.Retract}
	features := []string{}
	if file.Translate.Enabled {
		tr, err := translate.New(translate.Config{
			Provider: file.Translate.Provider,
			APIKey:   file.Translate.APIKey,
			APIURL:   file.Translate.APIURL,
			Model:    file.Translate.Model,
			FromLang: file.Translate.From,
		})
		if err != nil {
			logger.Fatal(err)
		}
		tc := file.Translate
		opt.Translate = &tc
		opt.Translator = tr
		features = append(features, "translate")
	}
	if file.Antispam.Enabled {
		opt.Antispam = antispam.New(file.Antispam)
		features = append(features, "antispam")
	}
	if file.Retract.Enabled {
		features = append(features, "retract")
	}
	logger.Printf("config=%s rooms=%d features=%v", configPath, len(rooms), features)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	cfg := appConfig{
		addr:   parsedAddr,
		pass:   file.XMPP.Password,
		opt:    opt,
		xmlIn:  xmlIn,
		xmlOut: xmlOut,
		logger: logger,
		debug:  debug,
	}
	if err := runLoop(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
		logger.Fatal(err)
	}
}

type appConfig struct {
	addr   jid.JID
	pass   string
	opt    bot.Options
	xmlIn  io.Writer
	xmlOut io.Writer
	logger *log.Logger
	debug  *log.Logger
}

func runLoop(ctx context.Context, cfg appConfig) error {
	smHandler := &sm.Handler{}
	var (
		attempt      int
		state        = reconnect.StateOffline
		connectStart time.Time
		dialServer   = cfg.addr.Domainpart()
	)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !reconnect.AttemptReconnect(state) {
			return fmt.Errorf("account state %v is not recoverable", state)
		}

		if !reconnect.ReconnectImmediately(state) {
			wait := reconnect.TimeToNextAttempt(
				attempt,
				time.Since(connectStart),
				reconnect.AggressiveBackoff(state),
				reconnect.PolicyViolationBackoff(state),
			)
			if wait > 0 {
				cfg.logger.Printf("reconnect in %s (attempt=%d state=%d)", wait.Round(time.Millisecond), attempt, state)
				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
			}
		}

		connectStart = time.Now()
		state = reconnect.StateConnecting
		cfg.logger.Printf("connecting (attempt=%d)…", attempt)

		sess, mucClient, err := dialSession(ctx, cfg, smHandler, dialServer)
		if err != nil {
			next, host := classifyDialError(err)
			if host != "" {
				dialServer = host
				cfg.logger.Printf("see-other-host: %s", host)
			}
			cfg.logger.Printf("connect failed: %v → %d", err, next)
			smHandler.ResetLive()
			if next == reconnect.StateUnauthorized || next == reconnect.StateIncompatibleServer {
				state = next
				return err
			}
			state = next
			attempt++
			if next == reconnect.StateConnectionTimeout {
				connectStart = time.Now()
			}
			continue
		}

		attempt = 0
		state = reconnect.StateOnline
		cfg.logger.Printf("online as %s (sm enabled=%v resumed=%v id=%q)",
			sess.LocalAddr(), smHandler.Enabled(), smHandler.Resumed(), smHandler.ID())

		lost := runSession(ctx, cfg, sess, mucClient, smHandler)
		_ = sess.Close()
		_ = sess.Conn().Close()
		smHandler.ResetLive()

		if errors.Is(lost, context.Canceled) || ctx.Err() != nil {
			return ctx.Err()
		}
		cfg.logger.Printf("connection lost: %v", lost)
		state = reconnect.StateOffline
		attempt++
		connectStart = time.Time{}
	}
}

func dialSession(
	ctx context.Context,
	cfg appConfig,
	smHandler *sm.Handler,
	server string,
) (*xmpp.Session, *muc.Client, error) {
	dialCtx, cancel := context.WithTimeout(ctx, reconnect.ConnectTimeout)
	defer cancel()

	d := dial.Dialer{}
	conn, err := d.DialServer(dialCtx, "tcp", cfg.addr, server)
	if err != nil {
		if dialCtx.Err() != nil && ctx.Err() == nil {
			return nil, nil, fmt.Errorf("%w: %v", errConnectTimeout, err)
		}
		return nil, nil, err
	}

	sess, err := xmpp.NewSession(dialCtx, cfg.addr.Domain(), cfg.addr, conn, 0, xmpp.NewNegotiator(func(*xmpp.Session, *xmpp.StreamConfig) xmpp.StreamConfig {
		return xmpp.StreamConfig{
			Lang: "en",
			Features: []xmpp.StreamFeature{
				xmpp.StartTLS(&tls.Config{
					ServerName: cfg.addr.Domain().String(),
					MinVersion: tls.VersionTLS12,
				}),
				xmpp.SASL("", cfg.pass, sasl.ScramSha1Plus, sasl.ScramSha1, sasl.Plain),
				smHandler.Feature(),
				xmpp.BindResource(),
			},
			TeeIn:  cfg.xmlIn,
			TeeOut: cfg.xmlOut,
		}
	}))
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}

	if !smHandler.Resumed() && smHandler.Supported() {
		if err := smHandler.Enable(dialCtx, sess); err != nil {
			cfg.logger.Printf("sm enable: %v (continuing without SM)", err)
		}
	}
	if !smHandler.Resumed() {
		smHandler.RememberBound(sess.LocalAddr())
	}

	return sess, &muc.Client{}, nil
}

var errConnectTimeout = errors.New("connect timeout")

func classifyDialError(err error) (reconnect.State, string) {
	if err == nil {
		return reconnect.StateOffline, ""
	}
	if errors.Is(err, errConnectTimeout) {
		return reconnect.StateConnectionTimeout, ""
	}
	var se stream.Error
	if errors.As(err, &se) {
		switch {
		case errors.Is(se, stream.SeeOtherHost):
			return reconnect.StateSeeOtherHost, se.Content
		case errors.Is(se, stream.PolicyViolation):
			return reconnect.StatePolicyViolation, ""
		case errors.Is(se, stream.NotAuthorized):
			return reconnect.StateUnauthorized, ""
		default:
			return reconnect.StateStreamError, ""
		}
	}
	var stanzaErr stanza.Error
	if errors.As(err, &stanzaErr) {
		switch stanzaErr.Condition {
		case stanza.NotAuthorized, stanza.NotAllowed, stanza.Forbidden:
			return reconnect.StateUnauthorized, ""
		}
	}
	if errors.Is(err, io.EOF) {
		return reconnect.StateOffline, ""
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return reconnect.StateConnectionTimeout, ""
		}
		return reconnect.StateUnreachable, ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "certificate"), strings.Contains(msg, "tls"):
		return reconnect.StateTLSError, ""
	case strings.Contains(msg, "auth"), strings.Contains(msg, "not-authorized"), strings.Contains(msg, "sasl"):
		return reconnect.StateUnauthorized, ""
	default:
		return reconnect.StateUnreachable, ""
	}
}

func runSession(
	ctx context.Context,
	cfg appConfig,
	sess *xmpp.Session,
	mucClient *muc.Client,
	smHandler *sm.Handler,
) error {
	rt := bot.New(ctx, sess, mucClient, smHandler, cfg.opt, cfg.logger, cfg.debug)
	stanzaMux := mux.New(stanza.NSClient, muc.HandleClient(mucClient), ping.Handle())

	var lastPacket time.Time
	notePacket := func() { lastPacket = time.Now() }

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- sess.Serve(smHandler.Wrap(xmpp.HandlerFunc(func(t xmlstream.TokenReadEncoder, start *xml.StartElement) error {
			// Application errors must not kill the stream; only I/O failures should.
			defer func() {
				if rec := recover(); rec != nil {
					cfg.logger.Printf("panic in serve handler: %v", rec)
				}
			}()
			notePacket()
			if start.Name.Local == "message" {
				d := xml.NewTokenDecoder(xmlstream.MultiReader(xmlstream.Token(*start), t))
				if _, err := d.Token(); err != nil {
					return err
				}
				var in peertrans.Incoming
				if err := d.DecodeElement(&in, start); err != nil && !errors.Is(err, io.EOF) {
					cfg.logger.Printf("decode message: %v", err)
					return nil
				}
				_ = rt.HandleIncoming(in) // always nil; never tear down Serve
				return nil
			}
			if err := stanzaMux.HandleXMPP(t, start); err != nil {
				cfg.logger.Printf("stanza handler: %v", err)
				return nil
			}
			return nil
		})))
	}()

	joinCtx, joinCancel := context.WithTimeout(ctx, 30*time.Second)
	defer joinCancel()
	if err := rt.JoinAll(joinCtx); err != nil {
		return err
	}
	notePacket()
	cfg.logger.Printf("joined %d room(s)", len(cfg.opt.Rooms))

	var lastPing time.Time
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-serveErr:
			if err == nil {
				return io.EOF
			}
			return err
		case now := <-ticker.C:
			if smHandler.Enabled() {
				if ack := smHandler.LastAck(); !ack.IsZero() && ack.After(lastPacket) {
					lastPacket = ack
				}
			}
			if reconnect.PingTimedOut(now, lastPacket, lastPing, reconnect.PingTimeout) {
				cfg.logger.Printf("ping/<r/> timeout; forcing reconnect")
				_ = sess.Conn().Close()
				select {
				case err := <-serveErr:
					return err
				case <-time.After(2 * time.Second):
					return fmt.Errorf("ping timeout")
				}
			}
			if !reconnect.PingIsDue(now, lastPacket, lastPing, reconnect.PingMinInterval) {
				continue
			}
			lastPing = now
			pingCtx, cancel := context.WithTimeout(ctx, reconnect.PingTimeout)
			var err error
			if smHandler.Enabled() {
				cfg.debug.Printf("sm request ack")
				err = smHandler.Request(pingCtx, sess)
			} else {
				cfg.debug.Printf("iq ping")
				err = ping.Send(pingCtx, sess, cfg.addr.Domain())
				if err == nil {
					notePacket()
				}
			}
			cancel()
			if err != nil {
				cfg.logger.Printf("liveness check failed: %v", err)
				_ = sess.Conn().Close()
				select {
				case serveE := <-serveErr:
					return serveE
				case <-time.After(2 * time.Second):
					return err
				}
			}
		}
	}
}
