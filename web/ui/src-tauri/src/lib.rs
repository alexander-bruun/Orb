use mdns_sd::{ServiceDaemon, ServiceEvent};
use serde::Serialize;
use std::time::Duration;

#[derive(Debug, Clone, Serialize)]
pub struct DiscoveredServer {
    pub name: String,
    pub host: String,
    pub port: u16,
    pub url: String,
    pub version: String,
}

#[tauri::command]
async fn discover_servers() -> Result<Vec<DiscoveredServer>, String> {
    let mdns = ServiceDaemon::new().map_err(|e| format!("mdns init: {e}"))?;
    let receiver = mdns
        .browse("_orb._tcp.local.")
        .map_err(|e| format!("mdns browse: {e}"))?;

    let mut servers: Vec<DiscoveredServer> = Vec::new();
    let deadline = tokio::time::Instant::now() + Duration::from_secs(3);

    loop {
        let remaining = deadline.saturating_duration_since(tokio::time::Instant::now());
        if remaining.is_zero() {
            break;
        }

        let rx = receiver.clone();
        let wait = remaining.min(Duration::from_millis(500));
        match tokio::task::spawn_blocking(move || rx.recv_timeout(wait)).await {
            Ok(Ok(ServiceEvent::ServiceResolved(info))) => {
                let host = info
                    .get_addresses()
                    .iter()
                    .next()
                    .map(|a| a.to_string())
                    .unwrap_or_else(|| {
                        info.get_hostname().trim_end_matches('.').to_string()
                    });
                let port = info.get_port();

                let props = info.get_properties();
                let path = props
                    .get("path")
                    .map(|v| v.val_str().to_string())
                    .unwrap_or_else(|| "/".to_string());
                let version = props
                    .get("version")
                    .map(|v| v.val_str().to_string())
                    .unwrap_or_else(|| "unknown".to_string());

                let url = format!("http://{}:{}{}", host, port, path);
                let name = info
                    .get_fullname()
                    .split('.')
                    .next()
                    .unwrap_or("Orb Server")
                    .to_string();

                if !servers.iter().any(|s| s.url == url) {
                    servers.push(DiscoveredServer {
                        name,
                        host,
                        port,
                        url,
                        version,
                    });
                }
            }
            _ => continue,
        }
    }

    let _ = mdns.stop_browse("_orb._tcp.local.");
    let _ = mdns.shutdown();

    Ok(servers)
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .invoke_handler(tauri::generate_handler![discover_servers])
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
