# Audiobook Library Structure

The audiobook ingester discovers files under configured roots. **Embedded tags are preferred**; folder names are used as fallbacks for series, title, and edition parsing.

## Supported layouts

### Series → Book → Chapters (recommended)

```text
Audiobooks/
  Series Name (by Author) - Subtitle (read by Narrator)/
    Book 1 - Title/
      01 - Chapter Title.mp3
      02 - Chapter Title.mp3
```

### Single-file audiobook (M4B/M4A)

```text
Audiobooks/
  Book Title/
    Book Title.m4b
```

A directory is treated as a single-file audiobook when it contains **exactly one** `.m4b` or `.m4a` and **no** multi-file formats.

Multi-file audiobooks can use `.mp3`, `.flac`, `.opus`, `.ogg`, `.aac`, `.wma`.

### Multi-part merge (Part/Disc folders)

If a directory has **no direct audio files**, **at least two** non-excluded subfolders, and **all** of those subfolders are part-like, those parts are merged into one audiobook:

```text
Audiobooks/
  Book Title/
    Part 1/
      01 - Chapter.mp3
    Part 2/
      01 - Chapter.mp3
```

Only audio files **directly inside** each part folder are included (nested folders are ignored).

Recognized part-like names: `Part N`, `Disc N`, `Disk N`, `CD N`, `Side N`, `Side A–D`.

## Folder naming rules

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

## Cover art lookup

- Embedded art in the first audio file is preferred.
- Otherwise, any `.jpg`, `.jpeg`, or `.png` in the book folder may be used; the most square image is chosen.
- For **multi-file** audiobooks, the parent folder is also checked for images.
