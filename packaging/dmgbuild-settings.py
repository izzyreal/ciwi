import os

format = "UDZO"
filesystem = "HFS+"
files = [(os.environ["CIWI_DMG_APP"], "Ciwi.app")]
symlinks = {"Applications": "/Applications"}
background = os.environ["CIWI_DMG_BACKGROUND"]
window_rect = ((120, 120), (780, 440))
show_toolbar = False
show_status_bar = False
show_tab_view = False
show_pathbar = False
show_sidebar = False
default_view = "icon-view"
arrange_by = None
icon_size = 128
text_size = 14
icon_locations = {
    "Ciwi.app": (190, 210),
    "Applications": (590, 210),
}

