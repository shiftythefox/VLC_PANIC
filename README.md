# VLC ESC Smiley v2.0

Apró Windows segédprogram VLC-hez.

## Funkció

- 20×20 pixeles smiley, mindig legfelül.
- Bal egérgombbal megfogható és arrébb húzható.
- Jobb egérgombbal bezárható.
- Globális `ESC` figyelés.
- Ha fut VLC és `ESC`-et nyomsz:
  - VLC kap egy `MEDIA_PAUSE` parancsot;
  - VLC minimalizálódik;
  - az adott ESC-et a program elnyeli.
- Ha VLC nem fut, az ESC normálisan működik.
- Nincs szükség Pythonra vagy külön futtatókörnyezetre.

## Mi változott a v2.0-ban?

A GUI teljesen újra lett írva minimál Win32 működésre.

A legfontosabb javítás: a GUI fő goroutine indulástól a message loop végéig ugyanahhoz az OS-threadhez van rögzítve (`runtime.LockOSThread`). A Win32 ablak és az üzenetsor thread-affine; a korábbi verziók ezt nem garantálták. Ez megmagyarázta azt a hibát, hogy az ablak látszott és az ESC-hook működött, de a smiley nem reagált egérre.

A húzásnál nincs saját `WM_MOUSEMOVE` logika: bal kattintáskor a program natív Windows ablakmozgatást indít (`WM_NCLBUTTONDOWN + HTCAPTION`).

Az ESC-hook továbbra is külön, saját OS-threaden fut.

Induláskor a v2.0 megpróbálja leállítani a korábban kiadott, pontosan ismert v1.x EXE-neveket, hogy ne maradjon a képernyőn egy régi hibás példány.

## Build

Windows + Go 1.23 vagy újabb:

```bat
build.bat
```

A kész fájl:

`release\VLC_ESC_Smiley_v2.exe`

## Forrás

`src/main.go`
