# TODO: GOGOGO!

1. Deprecate S3 support, i only want it to consume files mounted locally within the container. So remove all S3 and minio related things. And make sure we have a robust way to support many directories inside the container, since if the user has a lot of music across drives they wont all be inside the same directory.
2. Implement algorithm for suggested similar tracks. This should be used for user specific generated playlists based on their listen history. We should also use it for when a album or playlist runs out of tracks to play, afterwards recommended tracks should start playing. It should be based on the track audio similarity to other tracks in addition to comparing metadata.
3. If there is no song left in the personal queue, and its a song in the middle of a playlist or album where it was original started from, then playback should continue to the next song while taking shuffle mode into account.
4. Implement image resizing for the cover api endpoint with width and height if only one is provided then the other will be "auto", and update the home page to request a fixed size to eliminate any difference in cover art sizes so the cards are always the same size.
5. Implement a build for Tauri binaries and update the setup process to include a host configuration for where the server is hosted so the standalone app can find the backend from anywhere.
6. Investigate how we can bundle the backend and ingest in the same pod where N+1 pods are in secondary mode, and the primary is the only one running ingest duty. So we need to generate a quorom to determine the leader who writes to the database with newly ingested tracks / music. Or maybe we can use N+1 containers to distribute ingest load onto each replica like round robin. Where the primary pod distributes ingest load while only performing enrich duty since the API is rate limited.
7. Implement sound visualizers that are toggleable and moveable to a position on the screen the user desires.
8. Spectrum analyzer widget.
9. Waveform widget.
10. Discord Presence for native mac, linux and windows apps.
11. Add optional 4 digit code to the listen party, the host gets provided one at random if checkbox has been ticked in the session menu.
