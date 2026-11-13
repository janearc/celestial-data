//ninja:vow dojo it hands the whole-file walk to the tests outside
package spherex

// WholeBytes is the walk of a file that is FITS all the way through,
// for the tests outside the package: a whole FITS file that is not a
// cutout is still a whole walk, and the walk is tested on its own.
var WholeBytes = wholeBytes
