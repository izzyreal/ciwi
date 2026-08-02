Ciwi native client for Linux amd64

Run ./ciwi from this directory, or install it manually:

  - ciwi into a directory on PATH (for example ~/.local/bin)
  - ciwi.png into ~/.local/share/icons/hicolor/1024x1024/apps
  - ciwi.desktop into ~/.local/share/applications

The client discovers a local ciwi server automatically. To select a server
explicitly, start it with -addr host:port or set CIWI_NATIVE_SERVER.

This first Linux package uses Gio's X11/OpenGL backend. The host must provide
libX11, libX11-xcb, libXcursor, libXfixes, libxkbcommon, libxkbcommon-x11,
libEGL, and glibc. On Debian/Ubuntu these are available through packages such
as libx11-6, libx11-xcb1, libxcursor1, libxfixes3, libxkbcommon0,
libxkbcommon-x11-0, and libegl1.
