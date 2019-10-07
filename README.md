# [Blues Wireless][blues]

The note-go Go library for communicating with Blues Wireless Notecard via serial or I²C.

This library allows you to control a Notecard by coding in Go.
Your program may configure Notecard and send Notes to [Notehub.io][notehub].

See also:
* [note-c][note-c] for C bindings
* [note-python][note-python] for Python bindings

## Installing
For all releases, we have compiled the notecard utility for different OS and architectures [here](https://github.com/blues/note-go/releases).
If you don't see your OS and architecture supported, please file an issue and we'll add it to new releases.

[blues]: https://blues.com
[notehub]: https://notehub.io
[note-arduino]: https://github.com/blues/note-arduino
[note-c]: https://github.com/blues/note-c
[note-go]: https://github.com/blues/note-go
[note-python]: https://github.com/blues/note-python

## Dependencies
- Install Go and the Go tools [(here)](https://golang.org/doc/install)

## Compiling the notecard utility
If you want to build the latest, follow the directions below.
```bash
$ cd tools/notecard
$ go get -u .
$ go build .
```
