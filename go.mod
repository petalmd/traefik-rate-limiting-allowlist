module github.com/petalmd/traefik-rate-limiting-allowlist

go 1.15

require (
	github.com/mailgun/ttlmap v0.0.0-20170619185759-c1c17f74874f
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/stretchr/testify v1.7.0
	github.com/traefik/paerser v0.1.4
	github.com/traefik/traefik/v2 v2.4.8
	github.com/vulcand/oxy v1.3.0
	golang.org/x/time v0.0.0-20210611083556-38a9dc6acbc6
)

replace (
	github.com/abbot/go-http-auth => github.com/containous/go-http-auth v0.4.1-0.20200324110947-a37a7636d23e
	github.com/go-check/check => github.com/containous/check v0.0.0-20170915194414-ca0bf163426a
	github.com/gorilla/mux => github.com/containous/mux v0.0.0-20181024131434-c33f32e26898
	github.com/mailgun/minheap => github.com/containous/minheap v0.0.0-20190809180810-6e71eb837595
	github.com/mailgun/multibuf => github.com/containous/multibuf v0.0.0-20190809014333-8b6c9a7e6bba
)
