env "local" {
  src = "file://db/schema.sql"
  url = getenv("DATABASE_URL")
  dev = "docker://postgres/16/dev"
  migration {
    dir = "file://db/migrations"
  }
}
