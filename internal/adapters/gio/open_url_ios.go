//go:build ios

package gio

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework UIKit
#import <Foundation/Foundation.h>
#import <UIKit/UIKit.h>
#include <stdlib.h>

static void ciwiOpenURL(const char *rawURL) {
	NSString *value = [NSString stringWithUTF8String:rawURL];
	NSURL *url = [NSURL URLWithString:value];
	if (url == nil) {
		return;
	}
	dispatch_async(dispatch_get_main_queue(), ^{
		[[UIApplication sharedApplication] openURL:url options:@{} completionHandler:nil];
	});
}
*/
import "C"

import (
	"fmt"
	"net/url"
	"strings"
	"unsafe"
)

func openPlatformURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return fmt.Errorf("invalid web address")
	}
	value := C.CString(parsed.String())
	defer C.free(unsafe.Pointer(value))
	C.ciwiOpenURL(value)
	return nil
}
