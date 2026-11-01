module github.com/resonchat-org/resonbot

go 1.26.0

require (
	github.com/BurntSushi/toml v1.5.0
	mellium.im/sasl v0.3.2
	mellium.im/xmlstream v0.15.4
	mellium.im/xmpp v0.22.0
)

require (
	golang.org/x/crypto v0.55.0 // indirect
	golang.org/x/mod v0.41.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	golang.org/x/tools v0.49.0 // indirect
	mellium.im/reader v0.1.0 // indirect
)

replace mellium.im/xmpp => ./vendor/mellium_xmpp
