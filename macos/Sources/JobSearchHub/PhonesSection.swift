import CoreImage
import CoreImage.CIFilterBuiltins
import JobSearchHubCore
import SwiftUI

/// The phones paired with the hub: pair one through a QR code, and revoke
/// one that is lost.
struct PhonesSection: View {
    let client: HubClient
    @State private var devices: [Device] = []
    @State private var errorMessage: String?
    @State private var isPairing = false

    var body: some View {
        Section("Phones") {
            ForEach(devices) { device in
                HStack {
                    VStack(alignment: .leading, spacing: 2) {
                        Text(device.name)
                        Text(describe(device)).font(.caption).foregroundStyle(.secondary)
                    }
                    Spacer()
                    if device.revokedAt == nil {
                        Button("Revoke", role: .destructive) { Task { await revoke(device) } }
                    }
                }
            }
            if let errorMessage {
                Label(errorMessage, systemImage: "exclamationmark.triangle.fill").foregroundStyle(.orange)
            }
            Button("Pair a phone", systemImage: "qrcode") { isPairing = true }
        }
        .task { await load() }
        .sheet(isPresented: $isPairing, onDismiss: { Task { await load() } }) {
            PairPhoneSheet(client: client)
        }
    }

    private func describe(_ device: Device) -> String {
        if let revokedAt = device.revokedAt {
            return "Revoked \(revokedAt.formatted(date: .abbreviated, time: .omitted))"
        }
        if let lastSeenAt = device.lastSeenAt {
            return "Last seen \(lastSeenAt.formatted(.relative(presentation: .named)))"
        }
        return "Paired \(device.createdAt.formatted(date: .abbreviated, time: .omitted)), not seen yet"
    }

    private func load() async {
        do {
            devices = try await client.get("v1/devices", as: DevicesResponse.self).devices
            errorMessage = nil
        } catch {
            errorMessage = "Could not load the phones: \(error)"
        }
    }

    private func revoke(_ device: Device) async {
        do {
            try await client.delete("v1/devices/\(device.id)")
            await load()
        } catch {
            errorMessage = "Could not revoke \(device.name): \(error)"
        }
    }
}

/// Pairs a phone: its name and the address it reaches the hub at, then a QR
/// code with its new token, shown this once.
struct PairPhoneSheet: View {
    let client: HubClient
    @Environment(\.dismiss) private var dismiss
    @AppStorage("phoneHubAddress") private var phoneHubAddress = ""
    @State private var name = "My phone"
    @State private var link: String?
    @State private var errorMessage: String?
    @State private var isPairing = false
    @State private var isReadingTailscale = true

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Pair a phone").font(.title3.weight(.semibold))
            if let link {
                Text("Scan this with the Job Search Hub app on the phone. The token in it is shown only now.")
                    .foregroundStyle(.secondary)
                if let image = makeQRCode(link) {
                    Image(nsImage: image).interpolation(.none).resizable().frame(width: 260, height: 260)
                        .frame(maxWidth: .infinity)
                }
                Text(link).font(.caption.monospaced()).textSelection(.enabled).lineLimit(3)
                HStack {
                    Spacer()
                    Button("Done") { dismiss() }.keyboardShortcut(.defaultAction)
                }
            } else {
                TextField("Name", text: $name, prompt: Text("e.g. Tony's phone"))
                TextField("Address the phone uses", text: $phoneHubAddress, prompt: Text("https://<mac>.<tailnet>.ts.net, or http://10.0.2.2:8090 for the emulator"))
                Text(addressHint).font(.callout).foregroundStyle(.secondary)
                if let errorMessage {
                    Label(errorMessage, systemImage: "exclamationmark.triangle.fill").foregroundStyle(.orange)
                }
                HStack {
                    Spacer()
                    Button("Cancel") { dismiss() }
                    Button("Pair") { Task { await pair() } }
                        .keyboardShortcut(.defaultAction)
                        .disabled(isPairing || name.trimmingCharacters(in: .whitespaces).isEmpty || phoneHubAddress.trimmingCharacters(in: .whitespaces).isEmpty)
                }
            }
        }
        .padding(20)
        .frame(width: 520)
        .task { await fillInThisMacAddress() }
    }

    private var addressHint: String {
        let pairing = "Pair shows a QR code to scan with the Job Search Hub app on the phone."
        guard !isReadingTailscale, PhoneHubAddress.isUnreachableFromPhone(phoneHubAddress) else { return pairing }
        return "A phone can't reach the hub at localhost or the emulator's address. Use this Mac's Tailscale address: its name in the Tailscale menu, ending in .ts.net, with HTTPS certificates on in the Tailscale admin console. " + pairing
    }

    /// Starts the address at this Mac's Tailscale address, unless one a phone can reach is already there.
    private func fillInThisMacAddress() async {
        defer { isReadingTailscale = false }
        guard PhoneHubAddress.isUnreachableFromPhone(phoneHubAddress), let address = await TailscaleCommand.readThisMacAddress(),
              PhoneHubAddress.isUnreachableFromPhone(phoneHubAddress)
        else { return }
        phoneHubAddress = address
    }

    private func pair() async {
        isPairing = true
        defer { isPairing = false }
        do {
            let paired = try await client.send("POST", "v1/devices", body: PairDeviceRequest(name: name), as: PairDeviceResponse.self)
            link = PairingLink.make(hubURL: phoneHubAddress.trimmingCharacters(in: .whitespaces), token: paired.token)
            errorMessage = nil
        } catch {
            errorMessage = "Could not pair the phone: \(error)"
        }
    }

    private func makeQRCode(_ text: String) -> NSImage? {
        let filter = CIFilter.qrCodeGenerator()
        filter.message = Data(text.utf8)
        filter.correctionLevel = "M"
        guard let output = filter.outputImage else { return nil }
        let scaled = output.transformed(by: CGAffineTransform(scaleX: 10, y: 10))
        let representation = NSCIImageRep(ciImage: scaled)
        let image = NSImage(size: representation.size)
        image.addRepresentation(representation)
        return image
    }
}
