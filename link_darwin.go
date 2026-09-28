package main

// Wails references UTType without linking its framework.

// #cgo LDFLAGS: -framework UniformTypeIdentifiers
import "C"
