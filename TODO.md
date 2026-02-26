# TODO: GOGOGO!

If there is no song left in the personal queue, and its a song in the middle of a playlist or album where it was original started from, then playback should continue to the next song while taking shuffle mode into account.

Implement sound visualizers that are toggleable and moveable to a position on the screen the user desires.

Implement OpenLyrics for lyrics synced to the playing music, shown in a toggleable modal that can be moved around the screen by dragging it. It should go through the lyrics a line at a time, then go to the next line synced to the music.

Implement metadata retrieval from something like musicbrainz or discogs as we ingest and expand the database with more metadata information fields to enrich the experience.

Implement algorithm for suggested similar tracks. This should be used for user specific generated playlists based on their listen history. We should also use it for when a album or playlist runs out of tracks to play, afterwards recommended tracks should start playing. It should be based on the track audio similarity to other tracks in addition to comparing metadata.

Make sure to track the user's track listens so we can feature it on the home page. The home page should show the most recent played individual tracks, albums, playlists.

On the home page, add a recently added section showin the last N albums added.

Make the media playback controls be in line with other controls the shuffle and repeat option buttons are smaller icons, so even though they have the same padding they are not in line. So they need to be vertically centered.

Add a user page with user customizations and a personal settings panel. Here there should be a color scheme selector that works globally for the color scheme and dark / light mode maybe mult ui has a color scheme palette picker ui component?

Deprecate S3 support, i only want it to consume files mounted locally within the container. So remove all S3 and minio related things. And make sure we have a robust way to support many directories inside the container, since if the user has a lot of music across drives they wont all be inside the same directory.

Improve ingest performance, it is currently very slow to go through the data directory.

Implement a build for Tauri binaries and update the setup process to include a host configuration for where the server is hosted so the standalone app can find the backend from anywhere.

Something weird happens to the sound volume level when refreshing, it's not staying what it was when refreshing but it stays in the same visual position as before the refresh.

Investigate how we can bundle the backend and ingest in the same pod where N+1 pods are in secondary mode, and the primary is the only one running ingest duty. So we need to generate a quorom to determine the leader who writes to the database with newly ingested tracks / music. Or maybe we can use N+1 containers to distribute ingest load onto each replica like round robin. Where the primary pod distributes ingest load while also taking ingest load itself.
