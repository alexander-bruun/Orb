# Library Structure

This page documents how the ingester discovers files and which folder layouts yield the most reliable metadata for **audiobooks** and **music**. The ingester primarily trusts **embedded tags**; folder names are used as fallbacks and for edition parsing.

## Quick Take

- Any supported audio file found under configured roots is eligible for ingest.
- **Tags drive metadata** (Artist, Album Artist, Album, Title, Track/Disc, Year).
- Folder names help with **audiobook series/title parsing** and **album edition** detection.
- Cover art falls back to images in the same folder.

## Audiobooks

### Supported layouts

#### Series -> Book -> Chapters (recommended)

```text
Audiobooks/
  Series Name (by Author) - Subtitle (read by Narrator)/
    Book 1 - Title/
      01 - Chapter Title.mp3
      02 - Chapter Title.mp3
```

#### Single-file audiobook (M4B/M4A)

```text
Audiobooks/
  Book Title/
    Book Title.m4b
```

Rules:

- A directory is treated as a single-file audiobook when it contains **exactly one** `.m4b` or `.m4a` and **no** multi-file formats.
- Multi-file audiobooks can use `.mp3`, `.flac`, `.opus`, `.ogg`, `.aac`, `.wma`.

#### Multi-part merge (Part/Disc folders)

If a directory has **no direct audio files**, **at least two** non-excluded subfolders, and **all** of those subfolders are part-like, those parts are merged into one audiobook:

```text
Audiobooks/
  Book Title/
    Part 1/
      01 - Chapter.mp3
    Part 2/
      01 - Chapter.mp3
```

Notes:
- Only audio files **directly inside** each part folder are included (nested folders are ignored).

Recognized part-like names:

- `Part N`
- `Disc N`
- `Disk N`
- `CD N`
- `Side N` or `Side A-D`

### Folder naming rules (audiobooks)

Series folder patterns (trailing tags like `[FLAC]`, `(Unabridged)`, `(2019)` are ignored):

```text
Series Name (by Author) - Subtitle (read by Narrator)
Series Name by Author
Author - Series Name
```

Book folder patterns (edition tags and year suffixes are ignored):

```text
Book 1 - Title
#1 - Title
Title (Book 2)
01 - Title
01. Title
```

### Cover art lookup (audiobooks)

- Embedded art in the first audio file is preferred.
- Otherwise, any `.jpg`, `.jpeg`, or `.png` in the book folder may be used; the most square image is chosen.
- For **multi-file** audiobooks, the parent folder is also checked for images.

## Music

### Supported layouts

The music ingester walks configured roots and ingests any supported audio file at **any depth**. Folder names are only used for **edition parsing** and cover art fallback.

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

### Supported audio formats

- `.flac`, `.wav`, `.mp3`, `.aiff`, `.aif`

### Album folder naming

- Album **edition** is derived from bracketed tags in the **album directory name**, e.g. `[Deluxe]`, `{Remaster}`. Multiple bracketed tags are combined.
- Album identity is computed from **Album Artist tag + Album Title tag + album directory path**. Keeping editions in separate folders avoids collisions.
- For single top-level album folders like `Artist - Album (Year)`, ensure tags are set. Folder names are **not** parsed into artist/album.

### Cover art lookup (music)

- Embedded album art in audio files is preferred.
- Otherwise, any `.jpg`, `.jpeg`, or `.png` in the album folder may be used; the most square image is chosen.

## Tips for Best Metadata

- Ensure tags are filled: `Artist`, `Album Artist`, `Album`, `Title`, `Track Number`, `Disc Number`, `Year`.
- For multi-disc releases, set `Disc Number` tags rather than relying on folder names.
- Use consistent, numeric filename prefixes (`01 -`) to preserve order when tags are missing.
