// Thin wrappers over browser globals that jsdom cannot stub in tests.

export function reloadPage(): void {
  window.location.reload();
}
