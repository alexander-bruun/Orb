# Music Library Structure

The music ingester walks configured roots and ingests any supported audio file at **any depth**. **Tags drive metadata** — folder names are only used for edition parsing and cover art fallback.

## Supported layouts

All of these layouts are supported:

```text
Music/
  Artist Name/
    Album Title/
      01 - Track Title.flac
      02 - Track Title.flac
```

```text
Music/
  Artist Name/
    Album Title [Deluxe] {Remaster}/
      01 - Track Title.flac
```

```text
Music/
  Compilations/
    2020-07-24 Live Set/
      01 - Intro.wav
      02 - Track.wav
```

```text
Music/
  Daft Punk - Random Access Memories (2013)/
    01 - Give Life Back to Music.flac
    02 - The Game of Love.flac
```

```text
Music/
  Loose Tracks/
    random-file.mp3
```

## Supported audio formats

- `.flac`, `.wav`, `.mp3`, `.aiff`, `.aif`, `.dsf`, `.iso`

## Album folder naming

- Album **edition** is derived from bracketed tags in the **album directory name**, e.g. `[Deluxe]`, `{Remaster}`. Multiple bracketed tags are combined.
- Album identity is computed from **Album Artist tag + Album Title tag + album directory path**. Keeping editions in separate folders avoids collisions.
- For single top-level album folders like `Artist - Album (Year)`, ensure tags are set. Folder names are **not** parsed into artist/album.

## Cover art lookup

- Embedded album art in audio files is preferred.
- Otherwise, any `.jpg`, `.jpeg`, or `.png` in the album folder may be used; the most square image is chosen.

## Tips for best metadata

- Ensure tags are filled: `Artist`, `Album Artist`, `Album`, `Title`, `Track Number`, `Disc Number`, `Year`.
- For multi-disc releases, set `Disc Number` tags rather than relying on folder names.
- Use consistent, numeric filename prefixes (`01 -`) to preserve order when tags are missing.
