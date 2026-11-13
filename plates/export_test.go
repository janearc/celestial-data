//ninja:vow dojo it hands the plate check to the tests outside the package
package plates

// CheckPlate is the check a fetched plate is held to, for the tests
// outside the package: the plate Find read, and the blocks of header
// read at most.
func CheckPlate(plate string, blocks int, path string) error {
	return plateFile{plate: plate, blocks: blocks}.Check(path)
}

// ImageBytes and Padded are how the check sizes a plate's image, for the
// tests outside the package: the bytes a header declares, in a file so
// long, and a length made up to whole blocks.
var (
	ImageBytes = imageBytes
	Padded     = padded
)
