# TODO: GOGOGO!

If there is no song left in the personal queue, and its a song in the middle of a playlist or album where it was original started from, then playback should continue to the next song while taking shuffle mode into account.

Implement sound visualizers that are toggleable and moveable to a position on the screen the user desires.

Implement OpenLyrics for lyrics synced to the playing music, shown in a toggleable modal.

Implement metadata retrieval from something like musicbrainz or discogs as we ingest and expand the database with more metadata information fields to enrich the experience.

Implement algorithm for suggested similar tracks. This should be used for user specific generated playlists based on their listen history. We should also use it for when a album or playlist runs out of tracks to play, afterwards recommended tracks should start playing. It should be based on the track audio similarity to other tracks in addition to comparing metadata.

Add a badge to albums that are singles in the library viewer on top of the album cover art in the top right.

Add a favorites section to the left sidebar, that shows a page of the logged in users favorite tracks.

Make sure to track the user's track listens so we can feature it on the home page.

On the home page, add a recently added section showin the last N albums added.

Make the media playback controls be in line with other controls the shuffle and repeat option buttons are smaller icons, so even though they have the same padding they are not in line. So they need to be vertically centered.

Add a user page with user customizations and a personal settings panel. Here there should be a color scheme selector that works globally for the color scheme and dark / light mode.

Deprecate S3 support, i only want it to consume files mounted locally within the container. So remove all S3 and minio related things.

Improve ingest performance, it is currently very slow to go through the data directory.

Implement a build for Tauri binaries and update the setup process to include a host configuration for where the server is hosted so the standalone app can find the backend from anywhere. 

Something weird happens to the sound volume level when refreshing, it's not staying what it was when refreshing.
