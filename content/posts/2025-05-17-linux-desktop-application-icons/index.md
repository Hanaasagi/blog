+++
title = "Linux 下查找应用的 icon"
summary = ""
description = ""
categories = [""]
tags = []
date = 2025-05-17T11:03:28+09:00
draft = false

+++



## Desktop entries

根据 Freedesktop 规范，应用程序通常会创建一个 `.desktop` 文件，用于在桌面环境中注册自身。系统范围安装的应用程序，其 `.desktop` 文件通常位于 `/usr/share/applications/` 或 `/usr/local/share/applications/` 目录；而用户特定安装的应用程序，则通常位于 `~/.local/share/applications/`。在解析时，用户目录中的条目优先于系统目录中的条目。



文件格式如下，具体参考 https://specifications.freedesktop.org/desktop-entry-spec/latest/recognized-keys.html

```ini
[Desktop Entry]

# The type as listed above
Type=Application

# The version of the desktop entry specification to which this file complies
Version=1.0

# The name of the application
Name=jMemorize

# A comment which can/will be used as a tooltip
Comment=Flash card based learning tool

# The path to the folder in which the executable is run
Path=/opt/jmemorise

# The executable of the application, possibly with arguments.
Exec=jmemorize

# The name of the icon that will be used to display this entry
Icon=jmemorize

# Describes whether this application needs to be run in a terminal or not
Terminal=false

# Describes the categories in which this entry should be shown
Categories=Education;Languages;Java;

```



如 Firefox 的 `/usr/share/applications/firefox.desktop`

```ini
[Desktop Entry]
Version=1.0
Type=Application
Exec=/usr/lib/firefox/firefox %u
Terminal=false
X-MultipleArgs=false
Icon=firefox
StartupWMClass=firefox
DBusActivatable=false
Categories=GNOME;GTK;Network;WebBrowser;
MimeType=application/json;application/pdf;application/rdf+xml;application/rss+xml;application/x-xpinstall;application/xhtml+xml;application/xml;audio/flac;audio/ogg;audio/webm;image/avif;image/gif;image/jpeg;image/png;image/svg+xml;image/webp;text/html;text/xml;video/ogg;video/webm;x-scheme-handler/chrome;x-scheme-handler/http;x-scheme-handler/https;x-scheme-handler/mailto;
StartupNotify=true
Actions=new-window;new-private-window;open-profile-manager;
Name=Firefox
Name[zh_CN]=Firefox
Comment=Browse the World Wide Web
Comment[zh_CN]=浏览万维网
GenericName=Web Browser
GenericName[zh_CN]=Web 浏览器
Keywords=Internet;WWW;Browser;Web;Explorer;
Keywords[zh_CN]=Internet;WWW;Browser;Web;Explorer;
X-GNOME-FullName=Firefox Web Browser
X-GNOME-FullName[zh_CN]=Firefox 浏览器

[Desktop Action new-window]
Exec=/usr/lib/firefox/firefox --new-window %u
Name=New Window
Name[zh_CN]=新建窗口

[Desktop Action new-private-window]
Exec=/usr/lib/firefox/firefox --private-window %u
Name=New Private Window
Name[zh_CN]=新建隐私窗口

[Desktop Action open-profile-manager]
Exec=/usr/lib/firefox/firefox --ProfileManager
Name=Open Profile Manager
Name[zh_CN]=打开配置文件管理器
```





## Desktop icon

参考 https://specifications.freedesktop.org/icon-theme-spec/latest/。为了显示应用程序的图标，我们需要考虑

1. 图标路径
2. 文件的类型，`png` / `svg`
3. 系统主题，应用程序可能会提供一组图表用于不同的主题中
4. 缩放，`32x32`, `64x64` 不同大小，需要结合用户屏幕的 `dpi`



Freedesktop 规范规定了程序应该按照什么顺序和在哪些目录中查找图标：

1. `$HOME/.icons` (for backwards compatibility)
2. `$XDG_DATA_DIRS/icons`
3. `/usr/share/pixmaps`



路径查找首先在当前主题中进行，然后递归地在当前主题的每个父主题中进行，最后要求 fallback 到 `hicolor` 主题。因此想要安装应用程序图标，使其在 KDE 和 Gnome 菜单中正常工作，至少应该在 `hicolor` 中存放一组不同尺寸的图表。



获取当前主题可以通过 `gsettings` 命令，也可以读取 DConf 数据库来获取。参考 https://superuser.com/questions/581974/how-do-i-get-current-icon-theme-name-in-linux

```
$ gsettings get org.gnome.desktop.interface icon-theme
'McMojave-circle-dark'

$ gsettings get org.gnome.desktop.interface gtk-theme
'Breeze'

$ dconf read /org/gnome/desktop/interface/icon-theme
'McMojave-circle-dark'
```



比如 `Papirus` 这个主题，类似 Desktop Entry，它也有一个 `ini` 格式的配置文件，在 `/usr/share/icons/Papirus/index.theme` 。

```
[Icon Theme]
Name=Papirus
Comment=Papirus icon theme
Inherits=breeze,hicolor

```



`Inherits` 会告诉我们当前主题所继承的主题名称。同时这个文件中会声明不同大小的图标的目录的名称



- `/usr/share/icons/Papirus/32x32/apps/firefox.svg` ![](./Papirus-firefox-32.svg)

- `/usr/share/icons/hicolor/32x32/apps/firefox.png` ![](./hicolor-firefox-32.png)

- `/usr/share/icons/bloom-classic/apps/32/firefox.svg` ![](./bloom-classic-firefox-32.svg)



## Icon 路径查找实现

这里参考 Albert launcher 中的 C++ 实现。原始代码位于 https://github.com/albertlauncher/albert/blob/4f9c32b19b60ebc55393ab2111f263dd1c6582ea/src/platform/xdg/iconlookup.cpp



等价 Python 实现

```python
import configparser
import os
import subprocess
from pathlib import Path
from typing import List, Optional

ICON_EXTENSIONS = ["png", "svg", "xpm"]
FALLBACK_THEME = "hicolor"


class IconLookup:

    def __init__(self) -> None:
        self.icon_dirs = self._init_icon_dirs()
        self.icon_cache = {}

    def _init_icon_dirs(self) -> List[Path]:
        dirs = [
            Path.home() / ".icons",
            *map(
                lambda p: Path(p) / "icons",
                (
                    os.environ.get(
                        "XDG_DATA_DIRS", "/usr/local/share:/usr/share"
                    ).split(":")
                ),
            ),
            Path("/usr/local/share/pixmaps"),
            Path("/usr/share/pixmaps"),
        ]

        return [d for d in dirs if d.exists()]

    def icon_path(
        self,
        icon_name: str,
        *,
        theme_name: Optional[str] = None,
        desired_size: Optional[int] = None,
    ) -> Optional[str]:
        if not icon_name:
            return None

        if os.path.isabs(icon_name) and os.path.exists(icon_name):
            return icon_name

        for ext in ICON_EXTENSIONS:
            if icon_name.endswith(f".{ext}"):
                icon_name = icon_name[: -(len(ext) + 1)]

        cache_key = (icon_name, desired_size)
        if cache_key in self.icon_cache:
            return self.icon_cache[cache_key]

        checked_themes = []
        theme_name = theme_name or self.get_default_theme()

        # Theme lookup
        path = self._do_recursive_icon_lookup(
            icon_name, theme_name, checked_themes, desired_size
        )
        if path:
            self.icon_cache[cache_key] = path
            return path

        # Fallback to hicolor
        if FALLBACK_THEME not in checked_themes:
            path = self._do_recursive_icon_lookup(
                icon_name, FALLBACK_THEME, checked_themes, desired_size
            )
            if path:
                self.icon_cache[cache_key] = path
                return path

        # Unsorted search
        for icon_dir in self.icon_dirs:
            for ext in ICON_EXTENSIONS:
                candidate = icon_dir / f"{icon_name}.{ext}"
                if candidate.exists():
                    candidate_str = str(candidate)
                    self.icon_cache[cache_key] = candidate_str
                    return candidate_str

        self.icon_cache[cache_key] = None
        return None

    def _do_recursive_icon_lookup(
        self,
        icon_name: str,
        theme_name: str,
        checked: List[str],
        desired_size: Optional[int],
    ) -> Optional[str]:
        if theme_name in checked:
            return None
        checked.append(theme_name)

        theme_file = self._lookup_theme_file(theme_name)
        if not theme_file:
            return None

        path = self._do_icon_lookup(icon_name, theme_file, desired_size)
        if path:
            return path

        parents = self._parse_theme_parents(theme_file)
        for parent in parents:
            path = self._do_recursive_icon_lookup(
                icon_name, parent, checked, desired_size
            )
            if path:
                return path

        return None

    def _lookup_theme_file(self, theme_name: str) -> Optional[Path]:
        for icon_dir in self.icon_dirs:
            candidate = icon_dir / theme_name / "index.theme"
            if candidate.exists():
                return candidate
        return None

    def _do_icon_lookup(
        self, icon_name: str, theme_file: Path, desired_size: Optional[int]
    ) -> Optional[str]:
        parser = configparser.ConfigParser()
        parser.read(theme_file)
        theme_dir = theme_file.parent

        dirs = parser.get("Icon Theme", "Directories", fallback="").split(",")

        dirs_and_sizes = []
        for d in dirs:
            d = d.strip()
            size = parser.getint(d, "Size", fallback=0)
            dirs_and_sizes.append((d, size))

        if desired_size is not None:
            dirs_and_sizes.sort(key=lambda x: abs(x[1] - desired_size))
        else:
            dirs_and_sizes.sort(key=lambda x: -x[1])

        for subdir, _ in dirs_and_sizes:
            for icon_dir in self.icon_dirs:
                for ext in ICON_EXTENSIONS:
                    path = icon_dir / theme_dir.name / subdir / f"{icon_name}.{ext}"
                    if path.exists():
                        return str(path)
        return None

    def _parse_theme_parents(self, theme_file: Path) -> List[str]:
        parser = configparser.ConfigParser()
        parser.read(theme_file)
        inherits = parser.get("Icon Theme", "Inherits", fallback="")
        return [s.strip() for s in inherits.split(",") if s.strip()]

    def get_default_theme(self) -> str:
        try:
            return (
                subprocess.check_output(
                    [
                        "gsettings",
                        "get",
                        "org.gnome.desktop.interface",
                        "icon-theme",
                    ]
                )
                .decode()
                .strip("\r\n' ")
            )
        except subprocess.SubprocessError:
            return FALLBACK_THEME


if __name__ == "__main__":
    lookup = IconLookup()

    for name, size in [
        ("firefox", 64),
        ("firefox-nightly", 32),
        ("telegram-desktop", None),
        ("Alacritty", 32),
        ("albert", 32),
        ("electron", 32),
    ]:
        print(
            f"{name:<24}:",
            # lookup.icon_path(name, desired_size=size),
            lookup.icon_path(name, theme_name="bloom", desired_size=size),
        )


```



## Reference

- [Desktop entries - ArchLinux Wiki](https://wiki.archlinux.org/title/Desktop_entries)
-  [Icon Theme Specification](https://specifications.freedesktop.org/icon-theme-spec/latest/#index)
