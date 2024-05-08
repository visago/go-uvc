package uvc

/*
#include <libuvc-cgo.h>
*/
import "C"

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"unsafe"
)

// Auto-exposure mode.
type AEMode uint8

const (
	// manual exposure time, manual iris
	AEModeManual AEMode = 1
	// auto exposure time, auto iris
	AEModeAuto AEMode = 2
	// manual exposure time, auto iris
	AEModeShutterPriority AEMode = 4
	// auto exposure time, manual iris
	AEModeAperturePriority AEMode = 8

	// For SetAEPriority
	AEPriorityConstantFrameRate = 0
	AEPriorityVariableFrameRate = 1
)

var (
	ErrDeviceClosed   = errors.New("device closed")
	ErrDeviceNotFound = errors.New("device not found")
)

// VideoStreaming interface descriptor subtype.
type VSDescSubType C.enum_uvc_vs_desc_subtype

const (
	VS_UNDEFINED           VSDescSubType = C.UVC_VS_UNDEFINED
	VS_INPUT_HEADER        VSDescSubType = C.UVC_VS_INPUT_HEADER
	VS_OUTPUT_HEADER       VSDescSubType = C.UVC_VS_OUTPUT_HEADER
	VS_STILL_IMAGE_FRAME   VSDescSubType = C.UVC_VS_STILL_IMAGE_FRAME
	VS_FORMAT_UNCOMPRESSED VSDescSubType = C.UVC_VS_FORMAT_UNCOMPRESSED
	VS_FRAME_UNCOMPRESSED  VSDescSubType = C.UVC_VS_FRAME_UNCOMPRESSED
	VS_FORMAT_MJPEG        VSDescSubType = C.UVC_VS_FORMAT_MJPEG
	VS_FRAME_MJPEG         VSDescSubType = C.UVC_VS_FRAME_MJPEG
	VS_FORMAT_MPEG2TS      VSDescSubType = C.UVC_VS_FORMAT_MPEG2TS
	VS_FORMAT_DV           VSDescSubType = C.UVC_VS_FORMAT_DV
	VS_COLORFORMAT         VSDescSubType = C.UVC_VS_COLORFORMAT
	VS_FORMAT_FRAME_BASED  VSDescSubType = C.UVC_VS_FORMAT_FRAME_BASED
	VS_FRAME_FRAME_BASED   VSDescSubType = C.UVC_VS_FRAME_FRAME_BASED
	VS_FORMAT_STREAM_BASED VSDescSubType = C.UVC_VS_FORMAT_STREAM_BASED
)

type Device struct {
	dev    *C.uvc_device_t
	handle *C.uvc_device_handle_t
	mu     sync.RWMutex
}

// Ope opens a UVC device.
func (dev *Device) Open() error {
	dev.mu.Lock()
	defer dev.mu.Unlock()

	if dev.handle != nil {
		return nil
	}

	r := C.uvc_open(dev.dev, &dev.handle)
	if err := newError(ErrorType(r)); err != nil {
		return err
	}
	return nil
}

func (dev *Device) SetAEMode(mode AEMode) error {
	dev.mu.RLock()
	defer dev.mu.RUnlock()

	if dev.handle == nil {
		return ErrDeviceClosed
	}

	r := C.uvc_set_ae_mode(dev.handle, C.uchar(mode))
	return newError(ErrorType(r))
}

func (dev *Device) SetAEPriority(priority uint8) error {
	// 0: frame rate must remain constant; 1: frame rate may be varied for AE purposes
	// A `priority` value of zero means the camera may not vary its frame rate. A value of 1
	// means the frame rate is variable. This setting has no effect outside of the `auto` and
	// `shutter_priority` auto-exposure modes.
	dev.mu.RLock()
	defer dev.mu.RUnlock()

	if dev.handle == nil {
		return ErrDeviceClosed
	}

	r := C.uvc_set_ae_priority(dev.handle, C.uchar(priority))
	return newError(ErrorType(r))
}

func (dev *Device) SetExposureRelative(exposure int8) error {
	dev.mu.RLock()
	defer dev.mu.RUnlock()

	if dev.handle == nil {
		return ErrDeviceClosed
	}

	r := C.uvc_set_exposure_rel(dev.handle, C.schar(exposure))
	return newError(ErrorType(r))
}

func (dev *Device) SetBrightness(brightness int16) error {
	dev.mu.RLock()
	defer dev.mu.RUnlock()

	if dev.handle == nil {
		return ErrDeviceClosed
	}

	r := C.uvc_set_brightness(dev.handle, C.short(brightness))
	return newError(ErrorType(r))
}

// GetBusNumber gets the number of the bus to which the device is attached.
func (dev *Device) GetBusNumber() uint8 {
	return uint8(C.uvc_get_bus_number(dev.dev))
}

// GetAddress gets the number assigned to the device within its bus.
func (dev *Device) GetAddress() uint8 {
	return uint8(C.uvc_get_device_address(dev.dev))
}

func (dev *Device) ControlInterface() *ControlInterface {
	dev.mu.RLock()
	defer dev.mu.RUnlock()

	if dev.handle == nil {
		return nil
	}

	return &ControlInterface{
		itf:             dev.handle.info.ctrl_if,
		BcdUVC:          uint16(dev.handle.info.ctrl_if.bcdUVC),
		Number:          uint8(dev.handle.info.ctrl_if.bInterfaceNumber),
		EndpointAddress: uint8(dev.handle.info.ctrl_if.bEndpointAddress),
		ClockFrequency:  uint32(dev.handle.info.ctrl_if.dwClockFrequency),
	}
}

func (dev *Device) StreamInterfaces() (ifs []*StreamInterface) {
	dev.mu.RLock()
	defer dev.mu.RUnlock()

	if dev.handle == nil {
		return
	}

	for itf := dev.handle.info.stream_ifs; itf != nil; itf = itf.next {
		ifs = append(ifs, &StreamInterface{
			itf:             itf,
			Number:          uint8(itf.bInterfaceNumber),
			EndpointAddress: uint8(itf.bEndpointAddress),
			TerminalLink:    uint8(itf.bTerminalLink),
		})
	}
	return
}

func (dev *Device) Descriptor() (*DeviceDescriptor, error) {
	if dev.dev == nil {
		return nil, ErrDeviceNotFound
	}

	var desc *C.uvc_device_descriptor_t
	r := C.uvc_get_device_descriptor(dev.dev, &desc)
	if err := newError(ErrorType(r)); err != nil {
		return nil, err
	}
	defer C.uvc_free_device_descriptor(desc)

	info := &DeviceDescriptor{
		VendorID:     uint16(desc.idVendor),
		ProductID:    uint16(desc.idProduct),
		BcdUVC:       uint16(desc.bcdUVC),
		SerialNumber: C.GoString(desc.serialNumber),
		Manufacturer: C.GoString(desc.manufacturer),
		Product:      C.GoString(desc.product),
	}
	return info, nil
}

// GetStream gets a negotiated streaming control block for some common parameters.
func (dev *Device) GetStream(format FrameFormat, width, height, fps int) (*Stream, error) {
	dev.mu.RLock()
	defer dev.mu.RUnlock()

	if dev.handle == nil {
		return nil, ErrDeviceClosed
	}

	var ctrl C.uvc_stream_ctrl_t
	r := C.uvc_get_stream_ctrl_format_size(dev.handle, &ctrl,
		C.enum_uvc_frame_format(format),
		C.int(width), C.int(height), C.int(fps))
	if err := newError(ErrorType(r)); err != nil {
		return nil, err
	}
	return &Stream{
		devh: dev.handle,
		ctrl: ctrl,
	}, nil
}

// Ref increments the reference count for a device.
func (dev *Device) Ref() {
	dev.mu.RLock()
	defer dev.mu.RUnlock()

	if dev.handle == nil {
		return
	}

	C.uvc_ref_device(dev.dev)
}

// Unref decrements the reference count for a device.
func (dev *Device) Unref() {
	dev.mu.RLock()
	defer dev.mu.RUnlock()

	if dev.handle == nil {
		return
	}

	C.uvc_unref_device(dev.dev)
}

// Close closes a device.
// Ends any stream that's in progress.
// The device handle and frame structures will be invalidated.
func (dev *Device) Close() error {
	dev.mu.Lock()
	defer dev.mu.Unlock()

	C.uvc_close(dev.handle)
	dev.handle = nil

	return nil
}

func (dev *Device) IsIdle() bool {
	dev.mu.RLock()
	defer dev.mu.RUnlock()

	return dev.handle == nil || dev.handle.streams == nil
}

func (dev *Device) IsClosed() bool {
	dev.mu.RLock()
	defer dev.mu.RUnlock()

	return dev.handle == nil
}

type DeviceDescriptor struct {
	VendorID     uint16
	ProductID    uint16
	BcdUVC       uint16
	SerialNumber string
	Manufacturer string
	Product      string
}

func (d *DeviceDescriptor) String() string {
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "VendorID: %04x\n", d.VendorID)
	fmt.Fprintf(buf, "ProductID: %04x\n", d.ProductID)
	fmt.Fprintf(buf, "SerialNumber: %s\n", d.SerialNumber)
	fmt.Fprintf(buf, "BcdUVC: %d\n", d.BcdUVC)
	fmt.Fprintf(buf, "Manufacturer: %s\n", d.Manufacturer)
	fmt.Fprintf(buf, "Product: %s\n", d.Product)
	return buf.String()
}

// VideoControl interface.
type ControlInterface struct {
	itf             C.uvc_control_interface_t
	BcdUVC          uint16
	Number          uint8
	EndpointAddress uint8
	ClockFrequency  uint32
}

func (i *ControlInterface) String() string {
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "bcdUVC: %d\n", i.BcdUVC)
	fmt.Fprintf(buf, "InterfaceNumber: %d\n", i.Number)
	fmt.Fprintf(buf, "EndpointAddress: %d\n", i.EndpointAddress)
	fmt.Fprintf(buf, "ClockFrequency: %d\n", i.ClockFrequency)
	return buf.String()
}

// VideoStream interface.
type StreamInterface struct {
	itf *C.uvc_streaming_interface_t
	// Interface number
	Number uint8
	// USB endpoint to use when communicating with this interface
	EndpointAddress uint8
	TerminalLink    uint8
}

func (i *StreamInterface) FormatDescriptors() (descs []*FormatDescriptor) {
	if i.itf == nil {
		return
	}

	for desc := i.itf.format_descs; desc != nil; desc = desc.next {
		descs = append(descs, &FormatDescriptor{
			desc:                desc,
			Subtype:             VSDescSubType(desc.bDescriptorSubtype),
			FormatIndex:         uint8(desc.bFormatIndex),
			NumFrameDescriptors: uint8(desc.bNumFrameDescriptors),
			BitsPerPixel:        *(*uint8)(unsafe.Pointer(&desc.anon1[0])),
			Flags:               *(*uint8)(unsafe.Pointer(&desc.anon1[0])),
			DefaultFrameIndex:   uint8(desc.bDefaultFrameIndex),
			AspectRatioX:        uint8(desc.bAspectRatioX),
			AspectRatioY:        uint8(desc.bAspectRatioY),
			InterlaceFlags:      uint8(desc.bmInterlaceFlags),
			CopyProtect:         uint8(desc.bCopyProtect),
			VariableSize:        uint8(desc.bVariableSize),
		})
	}
	return
}

func (i *StreamInterface) String() string {
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "InterfaceNumber: %d\n", i.Number)
	fmt.Fprintf(buf, "EndpointAddress: %d\n", i.EndpointAddress)
	fmt.Fprintf(buf, "TerminalLink: %d\n", i.TerminalLink)

	return buf.String()
}

// device format descriptor.
// A "format" determines a stream's image type (e.g., raw YUYV or JPEG),
// and includes many "frame" configurations.
type FormatDescriptor struct {
	desc *C.uvc_format_desc_t
	// Type of image stream, such as JPEG or uncompressed.
	Subtype VSDescSubType
	// Identifier of this format within the VS interface's format list
	FormatIndex         uint8
	NumFrameDescriptors uint8
	// BPP for uncompressed stream
	BitsPerPixel uint8
	// Flags for JPEG stream
	Flags             uint8
	DefaultFrameIndex uint8
	AspectRatioX      uint8
	AspectRatioY      uint8
	InterlaceFlags    uint8
	CopyProtect       uint8
	VariableSize      uint8
}

func (d *FormatDescriptor) FrameDescriptors() (descs []*FrameDescriptor) {
	if d.desc == nil {
		return
	}
	for desc := d.desc.frame_descs; desc != nil; desc = desc.next {
		descs = append(descs, &FrameDescriptor{
			desc:                    desc,
			Subtype:                 VSDescSubType(desc.bDescriptorSubtype),
			FrameIndex:              uint8(desc.bFrameIndex),
			Capabilities:            uint8(desc.bmCapabilities),
			Width:                   uint16(desc.wWidth),
			Height:                  uint16(desc.wHeight),
			MinBitRate:              uint32(desc.dwMinBitRate),
			MaxBitRate:              uint32(desc.dwMaxBitRate),
			MaxVideoFrameBufferSize: uint32(desc.dwMaxVideoFrameBufferSize),
			DefaultFrameInterval:    uint32(desc.dwDefaultFrameInterval),
			MinFrameInterval:        uint32(desc.dwMinFrameInterval),
			MaxFrameInterval:        uint32(desc.dwMaxFrameInterval),
			FrameIntervalStep:       uint32(desc.dwFrameIntervalStep),
			FrameIntervalType:       uint8(desc.bFrameIntervalType),
			BytesPerLine:            uint32(desc.dwBytesPerLine),
			// Intervals: []uint32,
		})
	}
	return
}

func (d *FormatDescriptor) String() string {
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "Subtype: %d\n", d.Subtype)
	fmt.Fprintf(buf, "FormatIndex: %d\n", d.FormatIndex)
	fmt.Fprintf(buf, "NumFrameDescriptors: %d\n", d.NumFrameDescriptors)
	fmt.Fprintf(buf, "BitsPerPixel: %d\n", d.BitsPerPixel)
	fmt.Fprintf(buf, "Flags: %d\n", d.Flags)
	fmt.Fprintf(buf, "DefaultFrameIndex: %d\n", d.DefaultFrameIndex)
	fmt.Fprintf(buf, "AspectRatioX: %d\n", d.AspectRatioX)
	fmt.Fprintf(buf, "AspectRatioY: %d\n", d.AspectRatioY)
	fmt.Fprintf(buf, "InterlaceFlags: %d\n", d.InterlaceFlags)
	fmt.Fprintf(buf, "CopyProtect: %d\n", d.CopyProtect)
	fmt.Fprintf(buf, "VariableSize: %d\n", d.VariableSize)
	return buf.String()
}

// Frame descriptor.
// A "frame" is a configuration of a streaming format
// for a particular image size at one of possibly several available frame rates.
type FrameDescriptor struct {
	desc *C.uvc_frame_desc_t
	// Type of frame, such as JPEG frame or uncompressed frame
	Subtype VSDescSubType
	// Index of the frame within the list of specs available for this format
	FrameIndex   uint8
	Capabilities uint8
	// Image width
	Width uint16
	// Image height
	Height uint16
	// Bitrate of corresponding stream at minimal frame rate
	MinBitRate uint32
	// Bitrate of corresponding stream at maximal frame rate
	MaxBitRate uint32
	// Maximum number of bytes for a video frame
	MaxVideoFrameBufferSize uint32
	// Default frame interval (in 100ns units)
	DefaultFrameInterval uint32
	// Minimum frame interval for continuous mode (100ns units)
	MinFrameInterval uint32
	// Maximum frame interval for continuous mode (100ns units)
	MaxFrameInterval uint32
	// Granularity of frame interval range for continuous mode (100ns)
	FrameIntervalStep uint32
	// Frame intervals
	FrameIntervalType uint8
	// Number of bytes per line
	BytesPerLine uint32
	// Available frame rates, zero-terminated (in 100ns units)
	Intervals []uint32
}

func (d *FrameDescriptor) String() string {
	buf := &bytes.Buffer{}
	fmt.Fprintf(buf, "Subtype: %d\n", d.Subtype)
	fmt.Fprintf(buf, "FrameIndex: %d\n", d.FrameIndex)
	fmt.Fprintf(buf, "Capabilities: %d\n", d.Capabilities)
	fmt.Fprintf(buf, "Width: %d\n", d.Width)
	fmt.Fprintf(buf, "Height: %d\n", d.Height)
	fmt.Fprintf(buf, "MinBitRate: %d\n", d.MinBitRate)
	fmt.Fprintf(buf, "MaxBitRate: %d\n", d.MaxBitRate)
	fmt.Fprintf(buf, "MaxVideoFrameBufferSize: %d\n", d.MaxVideoFrameBufferSize)
	fmt.Fprintf(buf, "DefaultFrameInterval: %d\n", d.DefaultFrameInterval)
	fmt.Fprintf(buf, "MinFrameInterval: %d\n", d.MinFrameInterval)
	fmt.Fprintf(buf, "MaxFrameInterval: %d\n", d.MaxFrameInterval)
	fmt.Fprintf(buf, "FrameIntervalStep: %d\n", d.FrameIntervalStep)
	fmt.Fprintf(buf, "FrameIntervalType: %d\n", d.FrameIntervalType)
	fmt.Fprintf(buf, "BytesPerLine: %d\n", d.BytesPerLine)

	return buf.String()
}
