// Package spectrum measures a star's light in the SPHEREx cutouts the
// spherex archive fetched, one wavelength an image: together they are
// the star's spectrum from 0.75 to 5 microns.
//
// Each cutout is measured by forced photometry at the star's Gaia place,
// moved by its proper motion to the image's time, with the cutout's own
// effective point-spread function nearest the star.
//
// The background and the cutout's other peaks are fitted beside the
// star; a neighbour left out lends its light to it.
//
// It reads files and asks no one. A QR2 cutout carries the older PSF
// cube and no ePSF, and its fluxes are on QR2's gain, so it is refused,
// saying so, until SPHEREx's calibration files are fetched.
package spectrum
