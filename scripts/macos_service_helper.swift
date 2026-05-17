import Foundation
import ServiceManagement

let plistName = "nl.izmar.ciwi.agent.plist"

func usage() -> Never {
    fputs("usage: ciwi-service <register-agent|unregister-agent|status-agent>\n", stderr)
    exit(2)
}

guard CommandLine.arguments.count == 2 else {
    usage()
}

let service = SMAppService.agent(plistName: plistName)

do {
    switch CommandLine.arguments[1] {
    case "register-agent":
        if service.status == .enabled {
            try service.unregister()
        }
        try service.register()
        print("registered \(plistName)")
    case "unregister-agent":
        if service.status == .enabled {
            try service.unregister()
        }
        print("unregistered \(plistName)")
    case "status-agent":
        print("status=\(service.status.rawValue)")
    default:
        usage()
    }
} catch {
    fputs("ciwi-service: \(error)\n", stderr)
    exit(1)
}
