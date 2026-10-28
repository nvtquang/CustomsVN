/** Paths shown inside the settings frame instead of the chat layout. */
export function isSettingsPath(path: string) {
  return path === "/settings" || path.startsWith("/settings/") || path === "/admin" || path.startsWith("/admin/");
}
