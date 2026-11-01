// Copyright (C) 2026 ResonChat contributors.
//
// peertransbot joins a MUC and publishes plaintext Peer Message Translation
// (urn:resonchat:peertrans:0) replies for testing ResonChat merge.
//
// Reconnect / Stream Management follow Conversations (and ResonChat):
// getTimeToNextAttempt backoff, IQ ping or SM <r/>, XEP-0198 enable+resume.
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
	"sync"
	"time"

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

All settings are read from TOML (see config.example.toml). No environment
variables are used for account, room, or translation options.

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
	roomBare, err := jid.Parse(file.Room.JID)
	if err != nil {
		logger.Fatalf("room.jid: %v", err)
	}
	roomBare = roomBare.Bare()
	roomFull, err := roomBare.WithResource(file.Room.Nick)
	if err != nil {
		logger.Fatalf("room.nick: %v", err)
	}

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
	logger.Printf("config=%s translator=%s to=%v from=%s", configPath, tr.Name(), []string(file.Translate.To), file.Translate.From)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	cfg := botConfig{
		addr:       parsedAddr,
		pass:       file.XMPP.Password,
		roomBare:   roomBare,
		roomFull:   roomFull,
		roomPass:   file.Room.Password,
		priority:   file.Room.Priority,
		translator: tr,
		toLangs:    append([]string(nil), file.Translate.To...),
		fromLang:   file.Translate.From,
		xmlIn:      xmlIn,
		xmlOut:     xmlOut,
		logger:     logger,
		debug:      debug,
	}
	if err := runLoop(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
		logger.Fatal(err)
	}
}

type botConfig struct {
	addr       jid.JID
	pass       string
	roomBare   jid.JID
	roomFull   jid.JID
	roomPass   string
	priority   int
	translator translate.Translator
	toLangs    []string
	fromLang   string
	xmlIn      io.Writer
	xmlOut     io.Writer
	logger     *log.Logger
	debug      *log.Logger
}

func runLoop(ctx context.Context, cfg botConfig) error {
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
			// Conversations resets the connection clock on timeout so backoff
			// does not subtract the whole CONNECT_TIMEOUT.
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
		connectStart = time.Time{} // immediate offline reconnect
	}
}

func dialSession(
	ctx context.Context,
	cfg botConfig,
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
	cfg botConfig,
	sess *xmpp.Session,
	mucClient *muc.Client,
	smHandler *sm.Handler,
) error {
	stanzaMux := mux.New(stanza.NSClient, muc.HandleClient(mucClient), ping.Handle())
	b := &bot{
		session:    sess,
		sm:         smHandler,
		roomBare:   cfg.roomBare,
		nick:       cfg.roomFull.Resourcepart(),
		priority:   cfg.priority,
		translator: cfg.translator,
		toLangs:    cfg.toLangs,
		fromLang:   cfg.fromLang,
		logger:     cfg.logger,
		debug:      cfg.debug,
		onPacket:   nil, // set below
	}

	var lastPacket time.Time
	notePacket := func() { lastPacket = time.Now() }
	b.onPacket = notePacket

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- sess.Serve(smHandler.Wrap(xmpp.HandlerFunc(func(t xmlstream.TokenReadEncoder, start *xml.StartElement) error {
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
				return b.handle(in)
			}
			return stanzaMux.HandleXMPP(t, start)
		})))
	}()

	joinCtx, joinCancel := context.WithTimeout(ctx, 30*time.Second)
	defer joinCancel()
	opts := []muc.Option{muc.MaxBytes(0)}
	if cfg.roomPass != "" {
		opts = append(opts, muc.Password(cfg.roomPass))
	}
	cfg.logger.Printf("joining %s as %s…", cfg.roomBare, cfg.roomFull.Resourcepart())
	if _, err := mucClient.Join(joinCtx, cfg.roomFull, sess, opts...); err != nil {
		return fmt.Errorf("join: %w", err)
	}
	notePacket()
	cfg.logger.Printf("joined; translating plaintext groupchat messages")

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
			// SM <a/> updates LastAck on the serve goroutine; treat it as
			// packet activity (Conversations lastPacketReceived).
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

type bot struct {
	session    *xmpp.Session
	sm         *sm.Handler
	roomBare   jid.JID
	nick       string
	priority   int
	translator translate.Translator
	toLangs    []string
	fromLang   string
	logger     *log.Logger
	debug      *log.Logger
	onPacket   func()
}

func (b *bot) handle(in peertrans.Incoming) error {
	if b.onPacket != nil {
		b.onPacket()
	}
	if in.Type != stanza.GroupChatMessage {
		return nil
	}
	body := strings.TrimSpace(in.Body)
	if body == "" {
		return nil
	}
	if !in.From.Bare().Equal(b.roomBare) {
		return nil
	}
	if in.From.Resourcepart() == b.nick {
		return nil
	}
	if in.PeerTrans != nil {
		return nil
	}
	parentID := in.ParentID(b.roomBare)
	if parentID == "" {
		b.debug.Printf("skip message without usable id from %s", in.From)
		return nil
	}

	trCtx, trCancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer trCancel()

	type langHit struct {
		toLang string
		res    translate.Result
		err    error
	}
	hits := make([]langHit, len(b.toLangs))
	var wg sync.WaitGroup
	for i, toLang := range b.toLangs {
		wg.Add(1)
		go func(i int, toLang string) {
			defer wg.Done()
			res, err := b.translator.Translate(trCtx, body, b.fromLang, toLang)
			hits[i] = langHit{toLang: toLang, res: res, err: err}
		}(i, toLang)
	}
	wg.Wait()

	// Encode peertrans one at a time; mellium session is not concurrent-safe.
	for _, hit := range hits {
		if hit.err != nil {
			b.logger.Printf("translate → %s: %v", hit.toLang, hit.err)
			continue
		}
		res := hit.res
		src := res.SourceLang
		if src == "" {
			src = b.fromLang
		}
		if translate.SameLanguage(src, hit.toLang) || translate.SameLanguage(src, res.DestLang) {
			b.debug.Printf("skip %s: source already %s", hit.toLang, src)
			continue
		}
		originID := fmt.Sprintf("pt-%d", time.Now().UnixNano())
		out := peertrans.NewOutgoing(
			b.roomBare,
			in,
			res.DestLang,
			res.SourceLang,
			res.Text,
			res.Engine,
			b.priority,
			originID,
		)
		b.logger.Printf("peertrans %q → %q (%s engine=%s parent=%s from=%s)",
			body, res.Text, res.DestLang, res.Engine, parentID, in.From)
		sendCtx, sendCancel := context.WithTimeout(context.Background(), 15*time.Second)
		var err error
		if b.sm != nil && b.sm.Enabled() {
			err = b.sm.Encode(sendCtx, b.session, out)
		} else {
			err = b.session.Encode(sendCtx, out)
		}
		sendCancel()
		if err != nil {
			b.logger.Printf("send peertrans (%s): %v", res.DestLang, err)
		}
	}
	return nil
}
