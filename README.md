# ssainterp
A Golang interpreter, built upon [golang.org/x/tools/go/ssa/interp](https://godoc.org/golang.org/x/tools/go/ssa/interp). That original code contains the comment that "It is not, and will never be, a production-quality Go interpreter.", but that does not mean that it could not be of use in some very limited circumstances.

The planned use-case for this interpreter is where there is no access to a Go compiler in the production environment, for example in an embedded system or in Google App Engine.

### NOT PRODUCTION READY 

This code is a work-in-progress.
It is neither fully working nor properly documented, and is full of security vulnerabilities.

The tests are only known to pass on OSX & Ubuntu. The very simple benchmark suggests this interpreter runs 881x slower than compiled code, but 12x faster than the original implementation it was developed from.

Some ideas for future development were set-out in a [discussion document](https://docs.google.com/document/d/1Hvxf6NMPaCUd-1iqm_968SuHN1Vf8dLZQyHjvPyVE0Q/edit?usp=sharing).


