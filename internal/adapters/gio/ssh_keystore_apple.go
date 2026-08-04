//go:build (darwin || ios) && cgo

package gio

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdlib.h>
#include <string.h>

static CFMutableDictionaryRef ciwi_key_query(const char *service, const char *account) {
	CFMutableDictionaryRef query = CFDictionaryCreateMutable(NULL, 0, &kCFTypeDictionaryKeyCallBacks, &kCFTypeDictionaryValueCallBacks);
	CFStringRef serviceValue = CFStringCreateWithCString(NULL, service, kCFStringEncodingUTF8);
	CFStringRef accountValue = CFStringCreateWithCString(NULL, account, kCFStringEncodingUTF8);
	CFDictionarySetValue(query, kSecClass, kSecClassGenericPassword);
	CFDictionarySetValue(query, kSecAttrService, serviceValue);
	CFDictionarySetValue(query, kSecAttrAccount, accountValue);
	CFRelease(serviceValue);
	CFRelease(accountValue);
	return query;
}

static OSStatus ciwi_keychain_set(const char *service, const char *account, const void *bytes, int length) {
	CFMutableDictionaryRef query = ciwi_key_query(service, account);
	SecItemDelete(query);
	CFDataRef data = CFDataCreate(NULL, bytes, length);
	CFDictionarySetValue(query, kSecValueData, data);
	CFDictionarySetValue(query, kSecAttrAccessible, kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly);
	OSStatus status = SecItemAdd(query, NULL);
	CFRelease(data);
	CFRelease(query);
	return status;
}

static OSStatus ciwi_keychain_get(const char *service, const char *account, void **bytes, int *length) {
	CFMutableDictionaryRef query = ciwi_key_query(service, account);
	CFDictionarySetValue(query, kSecReturnData, kCFBooleanTrue);
	CFDictionarySetValue(query, kSecMatchLimit, kSecMatchLimitOne);
	CFTypeRef result = NULL;
	OSStatus status = SecItemCopyMatching(query, &result);
	CFRelease(query);
	if (status != errSecSuccess) return status;
	CFDataRef data = (CFDataRef)result;
	*length = (int)CFDataGetLength(data);
	*bytes = malloc(*length);
	if (*length > 0) memcpy(*bytes, CFDataGetBytePtr(data), *length);
	CFRelease(result);
	return errSecSuccess;
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

const sshKeychainService = "org.izzyreal.ciwi.native"

func loadSSHDevicePrivateKey(_ string) ([]byte, error) {
	service := C.CString(sshKeychainService)
	account := C.CString("ssh-device-key")
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	var bytes unsafe.Pointer
	var length C.int
	status := C.ciwi_keychain_get(service, account, &bytes, &length)
	if status == C.errSecItemNotFound {
		return nil, nil
	}
	if status != C.errSecSuccess {
		return nil, fmt.Errorf("read SSH device key from Keychain (status %d)", int(status))
	}
	defer C.free(bytes)
	return C.GoBytes(bytes, length), nil
}

func saveSSHDevicePrivateKey(_ string, privateKey []byte) error {
	service := C.CString(sshKeychainService)
	account := C.CString("ssh-device-key")
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(account))
	bytes := C.CBytes(privateKey)
	defer C.free(bytes)
	status := C.ciwi_keychain_set(service, account, bytes, C.int(len(privateKey)))
	if status != C.errSecSuccess {
		return fmt.Errorf("store SSH device key in Keychain (status %d)", int(status))
	}
	return nil
}
