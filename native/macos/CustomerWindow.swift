import Cocoa
import WebKit

// Native application shell. Only backend-announced loopback origins may navigate.
final class CustomerApp: NSObject, NSApplicationDelegate, WKNavigationDelegate {
    var window: NSWindow!
    var web: WKWebView!
    var backend: Process?
    var pending = Data()
    var origins = Set<String>()
    var quitting = false
    var loaded = false
    var layoutStarted = false
    var layoutResults = [[String: Any]]()
    func applicationDidFinishLaunching(_ notification: Notification) {
        let menu = NSMenu()
        let appItem = NSMenuItem(); menu.addItem(appItem)
        let appMenu = NSMenu(); appItem.submenu = appMenu
        appMenu.addItem(withTitle: "退出 PairShell", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        let editItem = NSMenuItem(); menu.addItem(editItem)
        let edit = NSMenu(title: "编辑"); editItem.submenu = edit
        for (title, action, key) in [("剪切", "cut:", "x"), ("复制", "copy:", "c"), ("粘贴", "paste:", "v"), ("全选", "selectAll:", "a")] {
            edit.addItem(withTitle: title, action: Selector(action), keyEquivalent: key)
        }
        NSApp.mainMenu = menu
        let config = WKWebViewConfiguration()
        config.websiteDataStore = .nonPersistent()
        web = WKWebView(frame: .zero, configuration: config)
        web.navigationDelegate = self
        window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 920, height: 720), styleMask: [.titled, .closable, .miniaturizable, .resizable], backing: .buffered, defer: false)
        window.title = "PairShell"
        window.minSize = NSSize(width: 640, height: 480)
        window.contentView = web; window.center(); window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
        let process = Process()
        process.executableURL = Bundle.main.bundleURL.appendingPathComponent("Contents/Helpers/PairShellBackend")
        var env = ProcessInfo.processInfo.environment
        env["PAIRSHELL_DESKTOP_EMBEDDED"] = "1"
        process.environment = env
        let pipe = Pipe(); process.standardOutput = pipe
        process.standardError = FileHandle.nullDevice
        pipe.fileHandleForReading.readabilityHandler = { [weak self] handle in
            let data = handle.availableData
            if data.isEmpty { handle.readabilityHandler = nil; return }
            DispatchQueue.main.async { self?.receive(data) }
        }
        process.terminationHandler = { [weak self] _ in
            DispatchQueue.main.async {
                guard let self = self else { return }
                if self.quitting { NSApp.reply(toApplicationShouldTerminate: true) }
                else { self.window.title = "PairShell · 已结束" }
            }
        }
        backend = process
        do { try process.run() } catch {
            let alert = NSAlert(); alert.messageText = "无法启动客户服务"; alert.informativeText = "请重新安装完整的 PairShell。"; alert.runModal(); NSApp.terminate(nil)
        }
    }
    func origin(_ url: URL) -> String? {
        guard url.scheme == "http", url.host == "127.0.0.1", let port = url.port, port > 0, url.user == nil, url.password == nil else { return nil }
        return "http://127.0.0.1:\(port)"
    }
    func receive(_ data: Data) {
        pending.append(data)
        if pending.count > 16384 { pending.removeAll(); return }
        while let newline = pending.firstIndex(of: 10) {
            let line = String(data: pending.prefix(upTo: newline), encoding: .utf8) ?? ""
            pending.removeSubrange(...newline)
            let prefix = "PAIRSHELL_WINDOW_URL="
            guard line.hasPrefix(prefix), let url = URL(string: String(line.dropFirst(prefix.count))), let value = origin(url) else { continue }
            origins.insert(value)
            // The initial page is loaded here; control URLs are announced before
            // the existing in-window JS redirects, preserving bootstrap ordering.
            if !loaded { loaded = true; var target=URLComponents(url:url,resolvingAgainstBaseURL:false)!; if (CommandLine.arguments.contains("--merchant") || Bundle.main.bundleIdentifier == "org.pairshell.merchant") {target.path="/merchant"}; web.load(URLRequest(url:target.url!)) }
        }
    }
    func webView(_ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction, decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        guard let url = navigationAction.request.url, let value = origin(url), origins.contains(value), navigationAction.targetFrame?.isMainFrame != false else { decisionHandler(.cancel); return }
        decisionHandler(.allow)
    }
    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
 window.title=webView.url?.path == "/merchant" ? "PairShell · 发起协助" : "PairShell · 接受协助"
        if CommandLine.arguments.contains("--layout-test") {
            if !layoutStarted { layoutStarted = true; runLayoutTest(0) }
            return
        }
        if CommandLine.arguments.contains("--smoke-test") {
            webView.evaluateJavaScript("document.querySelector('.brand')?.textContent || document.querySelector('h1')?.textContent") { value, error in
                if error == nil, let title = value as? String, title.contains("PairShell") {
                    print("WINDOW_SMOKE_READY: native window + embedded customer page")
                } else { print("WINDOW_SMOKE_FAILED") }
                NSApp.terminate(nil)
            }
        }
    }
    // Test-only, read-only DOM geometry. It never grants consent or modifies page content.
    func runLayoutTest(_ index: Int) {
        let sizes = [NSSize(width: 920, height: 720), NSSize(width: 700, height: 550), NSSize(width: 866, height: 660)]
        if index >= sizes.count {
            let passed = layoutResults.count == sizes.count && layoutResults.allSatisfy { ($0["passed"] as? Bool) == true }
            let result: [String: Any] = ["passed": passed, "views": layoutResults]
            if let data = try? JSONSerialization.data(withJSONObject: result, options: [.sortedKeys]), let json = String(data: data, encoding: .utf8) {
                print((passed ? "WINDOW_LAYOUT_READY " : "WINDOW_LAYOUT_FAILED ") + json)
                if let output = ProcessInfo.processInfo.environment["PAIRSHELL_LAYOUT_RESULT"] {
                    try? data.write(to: URL(fileURLWithPath: output + ".json"), options: .atomic)
                }
            } else { print("WINDOW_LAYOUT_FAILED: geometry serialization") }
            NSApp.terminate(nil)
            return
        }
        let size = sizes[index]
        window.setContentSize(size)
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.35) { [weak self] in
            guard let self = self else { return }
            let measurement = """
            (() => {const root=document.documentElement;const guideWidths=Array.from(document.querySelectorAll('.guide>div')).map(e=>e.getBoundingClientRect()).filter(r=>r.width>0&&r.height>0).map(r=>Math.round(r.width));return {viewportWidth:innerWidth,viewportHeight:innerHeight,clientWidth:root.clientWidth,scrollWidth:root.scrollWidth,scrollHeight:root.scrollHeight,verticalFits:root.scrollHeight<=innerHeight+1,guideWidths:guideWidths,passed:(document.querySelector('.workspace')!==null||innerWidth<800||guideWidths.length>0)&&guideWidths.every(w=>w>=160)&&root.scrollWidth<=root.clientWidth+1&&root.scrollHeight<=innerHeight+1};})()
            """
            self.web.evaluateJavaScript(measurement) { [weak self] value, error in
                guard let self = self else { return }
                var record = (value as? [String: Any]) ?? ["passed": false]
                record["requestedWidth"] = Int(size.width); record["requestedHeight"] = Int(size.height)
                if error != nil { record["passed"] = false; record["measurementError"] = true }
                self.layoutResults.append(record)
                guard let output = ProcessInfo.processInfo.environment["PAIRSHELL_LAYOUT_RESULT"] else { self.runLayoutTest(index + 1); return }
                let snapshot = WKSnapshotConfiguration()
                snapshot.rect = self.web.bounds
                self.web.takeSnapshot(with: snapshot) { [weak self] image, snapshotError in
                    guard let self = self else { return }
                    if snapshotError == nil, let tiff = image?.tiffRepresentation, let bitmap = NSBitmapImageRep(data: tiff), let png = bitmap.representation(using: .png, properties: [:]) {
                        try? png.write(to: URL(fileURLWithPath: output + "-" + String(Int(size.width)) + "x" + String(Int(size.height)) + ".png"), options: .atomic)
                    }
                    self.runLayoutTest(index + 1)
                }
            }
        }
    }
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }
    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        guard let process = backend, process.isRunning else { return .terminateNow }
        if quitting { return .terminateLater }
        quitting = true; process.terminate()
        DispatchQueue.main.asyncAfter(deadline: .now() + 12) { NSApp.reply(toApplicationShouldTerminate: true) }
        return .terminateLater
    }
}
let app = NSApplication.shared
app.setActivationPolicy(.regular)
let delegate = CustomerApp()
app.delegate = delegate
app.run()
