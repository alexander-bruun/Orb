#[cfg(desktop)]
mod desktop;

// ─── Entry Point ─────────────────────────────────────────────────────────────

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let builder = tauri::Builder::default();

    #[cfg(desktop)]
    #[cfg(desktop)]
    let builder = desktop::setup(builder);

    // On mobile (Android/iOS) use the local mediasession plugin which provides
    // the Android foreground service implementation. On other non-desktop
    // targets, fall back to the builder unchanged.
    #[cfg(all(not(desktop), mobile))]
    let builder = builder.plugin(tauri_plugin_mediasession::init());

    #[cfg(all(not(desktop), not(mobile)))]
    let builder = builder;

    // Mobile-only plugin registrations
    #[cfg(mobile)]
    let builder = builder.plugin(tauri_plugin_car::init());

    #[cfg(not(mobile))]
    let builder = builder;

    builder
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
