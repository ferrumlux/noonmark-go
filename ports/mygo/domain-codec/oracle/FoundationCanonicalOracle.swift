// SPDX-License-Identifier: AGPL-3.0-only
// Foundation formatter oracle ONLY. Does not validate Noonmark domain integrity.
import Foundation

// Only an explicitly supplied synthetic fixture is read. No app/runtime imports.
guard CommandLine.arguments.count == 2 else {
    throw NSError(domain: "NoonmarkFixtureOracle", code: 1,
                  userInfo: [NSLocalizedDescriptionKey: "Supply one synthetic fixture path"])
}
let fixture = URL(fileURLWithPath: CommandLine.arguments[1])
let data = try Data(contentsOf: fixture)
guard data.count <= 16 * 1024 * 1024 else {
    throw NSError(domain: "NoonmarkFixtureOracle", code: 2,
                  userInfo: [NSLocalizedDescriptionKey: "Fixture exceeds 16 MiB"])
}
let object = try JSONSerialization.jsonObject(with: data)
let output = try JSONSerialization.data(withJSONObject: object,
                                      options: [.prettyPrinted, .sortedKeys, .withoutEscapingSlashes])
FileHandle.standardOutput.write(output)
