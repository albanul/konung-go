Konung 1.5 - a remake of «Князь: Легенды лесной страны» (1999)
================================================================

This folder has the game and the network game's server. The original
game's files (pictures, maps, sounds) are not included: you need your own
copy of the original game.


1. PUT THE GAME FILES IN
------------------------

Copy everything from the original game's folder (the one with konung.exe,
GRAPH.RES, KONUNG.1 ...) into the folder assets/Konung here:

    this folder/
      konung            (the game; konung.exe on Windows)
      konung-server     (the network game's server)
      assets/Konung/    <- the game's files go here
      web/
      README.txt


2. PLAY ALONE
-------------

Start "konung" (Windows: konung.exe). The game opens fullscreen.
Your saves and settings are kept in assets/Konung.


3. PLAY TOGETHER ON YOUR NETWORK
--------------------------------

One computer starts the server: "konung-server" (Windows:
konung-server.exe). A window opens and shows what the server does; keep it
open while you play (closing it stops the server).

Then everyone plays:

  * in a web browser, nothing to install: open
        http://localhost:7777/       on the server's computer
        http://<its address>:7777/   on the others (for example
                                     http://192.168.1.20:7777/)
    The address: on Windows run "ipconfig" in a Command Prompt
    (the "IPv4 Address"); on macOS: System Settings -> Network;
    on Linux: "hostname -I" in a terminal.

  * or in the game: menu «Сетевая игра» -> «Новая игра». The server is
    found by itself; otherwise type its address and press Enter.

Log in with a name and a password (the first time creates your account),
click a hero's portrait (it decides your team) and press Ok.

The others must reach port 7777 on the server's computer: allow it when the
system's firewall asks.

Settings (server name, players, session length ...): copy
konung-server.conf.example to konung-server.conf next to the server, edit
it, and start the server again. Accounts and progress are kept in the folder
server-data.


IF SOMETHING DOES NOT WORK
--------------------------

* macOS says the program "cannot be opened": right-click it -> Open -> Open
  (only the first time). If it says "damaged", run in Terminal:
      xattr -dr com.apple.quarantine <this folder>
* Linux, "error while loading shared libraries": install the graphics and
  sound libraries (Fedora: sudo dnf install mesa-libGL alsa-lib;
  Ubuntu: sudo apt install libgl1 libasound2).
* The game does not start / no «Сетевая игра» in the menu: some game files
  are missing in assets/Konung (the network game needs LAN_KONG.EXE,
  ME800R.RES, PLAY800R.RES, MAPS.RES, GAME.RES ... from the original's
  network version).
* The films are skipped: they need a one-time conversion; see the
  project's README on GitHub.

More: https://github.com/albanul/konung-go
