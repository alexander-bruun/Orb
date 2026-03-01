module github.com/alexander-bruun/orb/services/api

go 1.26.0

require (
	github.com/alexander-bruun/orb/pkg/config v0.0.0-00010101000000-000000000000
	github.com/alexander-bruun/orb/pkg/kvkeys v0.0.0-00010101000000-000000000000
	github.com/alexander-bruun/orb/pkg/objstore v0.0.0-00010101000000-000000000000
	github.com/alexander-bruun/orb/pkg/store v0.0.0-00010101000000-000000000000
	github.com/go-chi/chi/v5 v5.2.5
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	github.com/gorilla/websocket v1.5.3
	github.com/hashicorp/mdns v1.0.6
	github.com/jackc/pgx/v5 v5.8.0
	github.com/redis/go-redis/v9 v9.18.0
	golang.org/x/crypto v0.48.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/klauspost/cpuid/v2 v2.2.11 // indirect
	github.com/miekg/dns v1.1.55 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/mod v0.32.0 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	golang.org/x/tools v0.41.0 // indirect
)

replace (
	github.com/alexander-bruun/orb/pkg/config => ../../pkg/config
	github.com/alexander-bruun/orb/pkg/kvkeys => ../../pkg/kvkeys
	github.com/alexander-bruun/orb/pkg/objstore => ../../pkg/objstore
	github.com/alexander-bruun/orb/pkg/store => ../../pkg/store
)
