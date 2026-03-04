module github.com/alexander-bruun/orb/cmd/ingest

go 1.26.0

require (
	github.com/alexander-bruun/orb/pkg/config v0.0.0-00010101000000-000000000000
	github.com/alexander-bruun/orb/pkg/musicbrainz v0.0.0-00010101000000-000000000000
	github.com/alexander-bruun/orb/pkg/objstore v0.0.0-00010101000000-000000000000
	github.com/alexander-bruun/orb/pkg/similarity v0.0.0-00010101000000-000000000000
	github.com/alexander-bruun/orb/pkg/store v0.0.0-00010101000000-000000000000
	github.com/dhowden/tag v0.0.0-20240417053706-3d75831295e8
	github.com/fsnotify/fsnotify v1.9.0
	github.com/spf13/cobra v1.10.2
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.8.0 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
)

replace (
	github.com/alexander-bruun/orb/pkg/config => ../../pkg/config
	github.com/alexander-bruun/orb/pkg/musicbrainz => ../../pkg/musicbrainz
	github.com/alexander-bruun/orb/pkg/objstore => ../../pkg/objstore
	github.com/alexander-bruun/orb/pkg/similarity => ../../pkg/similarity
	github.com/alexander-bruun/orb/pkg/store => ../../pkg/store
)
