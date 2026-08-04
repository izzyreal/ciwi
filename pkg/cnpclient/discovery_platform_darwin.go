//go:build darwin && cgo

package cnpclient

/*
#include <dns_sd.h>
#include <arpa/inet.h>
#include <stdlib.h>
#include <string.h>
#include <sys/select.h>
#include <sys/time.h>

#define CIWI_DNSSD_MAX_ENTRIES 16
#define CIWI_DNSSD_NAME_SIZE 256
#define CIWI_DNSSD_HOST_SIZE 256
#define CIWI_DNSSD_TXT_SIZE 1024

typedef struct {
	uint32_t interface_index;
	char name[CIWI_DNSSD_NAME_SIZE];
	char regtype[CIWI_DNSSD_NAME_SIZE];
	char domain[CIWI_DNSSD_NAME_SIZE];
} ciwi_browse_entry;

typedef struct {
	char name[CIWI_DNSSD_NAME_SIZE];
	char host[CIWI_DNSSD_HOST_SIZE];
	char txt[CIWI_DNSSD_TXT_SIZE];
	uint16_t port;
} ciwi_dnssd_entry;

typedef struct {
	ciwi_browse_entry entries[CIWI_DNSSD_MAX_ENTRIES];
	int count;
	int error;
	int more_coming;
} ciwi_browse_context;

typedef struct {
	ciwi_dnssd_entry *entry;
	int complete;
	int error;
} ciwi_resolve_context;

static int64_t ciwi_now_millis(void) {
	struct timeval value;
	gettimeofday(&value, NULL);
	return ((int64_t)value.tv_sec * 1000) + (value.tv_usec / 1000);
}

static int ciwi_process_one(DNSServiceRef ref, int timeout_ms) {
	int fd = DNSServiceRefSockFD(ref);
	if (fd < 0) return -1;
	fd_set reads;
	FD_ZERO(&reads);
	FD_SET(fd, &reads);
	struct timeval timeout;
	timeout.tv_sec = timeout_ms / 1000;
	timeout.tv_usec = (timeout_ms % 1000) * 1000;
	int selected = select(fd + 1, &reads, NULL, NULL, &timeout);
	if (selected <= 0) return selected;
	return DNSServiceProcessResult(ref) == kDNSServiceErr_NoError ? 1 : -1;
}

static void ciwi_browse_callback(
	DNSServiceRef ref, DNSServiceFlags flags, uint32_t interface_index,
	DNSServiceErrorType error_code, const char *service_name,
	const char *regtype, const char *reply_domain, void *opaque
) {
	(void)ref;
	ciwi_browse_context *context = (ciwi_browse_context *)opaque;
	if (error_code != kDNSServiceErr_NoError) {
		context->error = (int)error_code;
		return;
	}
	context->more_coming = (flags & kDNSServiceFlagsMoreComing) != 0;
	if ((flags & kDNSServiceFlagsAdd) == 0 || context->count >= CIWI_DNSSD_MAX_ENTRIES) return;
	for (int index = 0; index < context->count; index++) {
		ciwi_browse_entry *known = &context->entries[index];
		if (known->interface_index == interface_index && strcmp(known->name, service_name) == 0 &&
			strcmp(known->regtype, regtype) == 0 && strcmp(known->domain, reply_domain) == 0) return;
	}
	ciwi_browse_entry *entry = &context->entries[context->count++];
	entry->interface_index = interface_index;
	strlcpy(entry->name, service_name, sizeof(entry->name));
	strlcpy(entry->regtype, regtype, sizeof(entry->regtype));
	strlcpy(entry->domain, reply_domain, sizeof(entry->domain));
}

static void ciwi_resolve_callback(
	DNSServiceRef ref, DNSServiceFlags flags, uint32_t interface_index,
	DNSServiceErrorType error_code, const char *fullname, const char *hosttarget,
	uint16_t port, uint16_t txt_length, const unsigned char *txt_record, void *opaque
) {
	(void)ref; (void)flags; (void)interface_index; (void)fullname;
	ciwi_resolve_context *context = (ciwi_resolve_context *)opaque;
	if (error_code != kDNSServiceErr_NoError) {
		context->error = (int)error_code;
		context->complete = 1;
		return;
	}
	strlcpy(context->entry->host, hosttarget, sizeof(context->entry->host));
	context->entry->port = ntohs(port);
	size_t source = 0;
	size_t target = 0;
	while (source < txt_length && target + 1 < sizeof(context->entry->txt)) {
		unsigned int field_length = txt_record[source++];
		if (source + field_length > txt_length) break;
		size_t available = sizeof(context->entry->txt) - target - 1;
		size_t copied = field_length < available ? field_length : available;
		memcpy(context->entry->txt + target, txt_record + source, copied);
		target += copied;
		source += field_length;
		if (target + 1 < sizeof(context->entry->txt)) context->entry->txt[target++] = '\n';
	}
	context->entry->txt[target] = '\0';
	context->complete = 1;
}

static int ciwi_dnssd_browse(const char *service, int timeout_ms, ciwi_dnssd_entry *output, int capacity) {
	if (capacity <= 0) return 0;
	int64_t deadline = ciwi_now_millis() + timeout_ms;
	ciwi_browse_context browse;
	memset(&browse, 0, sizeof(browse));
	DNSServiceRef browser = NULL;
	DNSServiceErrorType started = DNSServiceBrowse(
		&browser, 0, 0, service, NULL, ciwi_browse_callback, &browse
	);
	if (started != kDNSServiceErr_NoError) return (int)started;

	while (ciwi_now_millis() < deadline && browse.error == 0) {
		int remaining = (int)(deadline - ciwi_now_millis());
		int processed = ciwi_process_one(browser, remaining);
		if (processed <= 0) break;
		if (browse.count > 0 && !browse.more_coming) break;
	}
	DNSServiceRefDeallocate(browser);
	if (browse.error != 0) return browse.error;

	int count = browse.count < capacity ? browse.count : capacity;
	for (int index = 0; index < count; index++) {
		ciwi_browse_entry *source = &browse.entries[index];
		ciwi_dnssd_entry *target = &output[index];
		memset(target, 0, sizeof(*target));
		strlcpy(target->name, source->name, sizeof(target->name));
		DNSServiceRef resolver = NULL;
		ciwi_resolve_context resolve;
		memset(&resolve, 0, sizeof(resolve));
		resolve.entry = target;
		DNSServiceErrorType resolving = DNSServiceResolve(
			&resolver, 0, source->interface_index, source->name, source->regtype,
			source->domain, ciwi_resolve_callback, &resolve
		);
		if (resolving != kDNSServiceErr_NoError) continue;
		while (!resolve.complete && resolve.error == 0) {
			int remaining = (int)(deadline - ciwi_now_millis());
			if (remaining <= 0) break;
			int processed = ciwi_process_one(resolver, remaining);
			if (processed <= 0) break;
		}
		DNSServiceRefDeallocate(resolver);
		if (!resolve.complete || resolve.error != 0) target->port = 0;
	}
	return count;
}
*/
import "C"

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
	"unsafe"
)

func discoverPlatformService(ctx context.Context, service string, timeout time.Duration) ([]*discoveryEntry, bool, error) {
	if timeout <= 0 {
		timeout = time.Second
	}
	serviceName := C.CString(service)
	defer C.free(unsafe.Pointer(serviceName))
	entries := make([]C.ciwi_dnssd_entry, C.CIWI_DNSSD_MAX_ENTRIES)
	timeoutMilliseconds := timeout.Milliseconds()
	if timeoutMilliseconds < 1 {
		timeoutMilliseconds = 1
	}
	count := int(C.ciwi_dnssd_browse(
		serviceName,
		C.int(timeoutMilliseconds),
		&entries[0],
		C.int(len(entries)),
	))
	if count < 0 {
		if count == int(C.kDNSServiceErr_PolicyDenied) {
			return nil, true, fmt.Errorf("Bonjour discovery is not permitted; allow Local Network access for Ciwi in Settings")
		}
		return nil, true, fmt.Errorf("browse for %s with Bonjour: DNS-SD error %d", service, count)
	}
	result := make([]*discoveryEntry, 0, count)
	for index := 0; index < count; index++ {
		entry := &entries[index]
		port := int(entry.port)
		host := C.GoString(&entry.host[0])
		if port <= 0 || host == "" {
			continue
		}
		resolved := &discoveryEntry{
			Name:       C.GoString(&entry.name[0]),
			Host:       host,
			Port:       port,
			InfoFields: compactTXTFields(C.GoString(&entry.txt[0])),
		}
		lookupCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
		addresses, err := net.DefaultResolver.LookupIPAddr(lookupCtx, strings.TrimSuffix(host, "."))
		cancel()
		if err == nil {
			for _, address := range addresses {
				if address.IP.To4() != nil {
					resolved.AddrIPv4 = append(resolved.AddrIPv4, address.IP)
				} else {
					resolved.AddrIPv6 = append(resolved.AddrIPv6, address.IP)
				}
			}
		}
		result = append(result, resolved)
	}
	return result, true, ctx.Err()
}

func compactTXTFields(value string) []string {
	lines := strings.Split(value, "\n")
	result := lines[:0]
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			result = append(result, line)
		}
	}
	return result
}
