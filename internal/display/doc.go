// Package display is deterministic black/white/red/yellow raster drawing, and
// knows nothing about what the raster is for.
//
// It owns stateful canvases, reusable display lists, primitives, paths, bitmap
// fonts, text and image rendering, and the PNG preview. It does not own a page
// size, a panel or a tag family: a frame is made at a size somebody states, and
// the two wire encoders live beside the protocols that want them, in gicisky
// and nrfepd. This used to be otherwise — there was a default page here that
// was one family's 2.9" panel, and an encoder for that family beside it.
package display
