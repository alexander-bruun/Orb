# Feature Ideas

- "Radio" mode — infinite queue seeded from a track or artist using the existing `recommend` service, auto-fetching more tracks as the queue drains.
- Add native make targets for building to Windows, Linux and Mac like Android and iOS so i dont have to rely on pipeline built binaries locally for testing.
- Auto-playlist (smart playlist) — saved filter rules (genre = "Jazz", year > 2000, play count < 5) that dynamically populate a playlist on open. These should be shown on the home page, and refresh once a week, and once a day when it makes sense.
- iOS Support: Start developing iOS app using Tauri v2
- On-the-fly transcoding / bitrate limiting for bandwidth-constrained devices based on the users settings.
- Track ratings (1–5 stars) — finer-grained than a binary favorite; enables weighted recommendations. Add a star in addition to favorite button, that shows 5 stars popping out of the star around it, then the user can put a rating from 1-5.
- Rework the admin settings base url depending on the platform, if on mobile it is aware of this due to the setup. On the website it should use the domain of the website. This way the register url that is sent out should work properly.
- Lazy load cover art on all pages where many cover arts are shown to reduce cover art retrieval spam when navigating.
- On mobile the library page should not stick the character i have scrolled to since it leaves a gap at the top of the screen.
- On mobile, undertneith the minimzed media player island, i cant see through between the bottom menu and the island, it's as if it has a background. I was expectecting to be able to see the page's content in the gap between the island and menu.
- Download song lyrics and the song wave for the seek bar offline in addition to the song and cover art.