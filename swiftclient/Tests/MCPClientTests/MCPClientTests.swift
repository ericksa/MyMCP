import XCTest
@testable import MCPClient

final class MCPClientTests: XCTestCase {
    func testConfigDefaultValues() {
        let config = MCPConfig()
        XCTAssertEqual(config.serverURL, "http://localhost:8080/mcp")
        XCTAssertEqual(config.model, "llama3")
    }
    
    func testMCPMessage() {
        let message = MCPMessage(role: "user", content: "Hello")
        XCTAssertEqual(message.role, "user")
        XCTAssertEqual(message.content, "Hello")
    }
    
    func testMCPTool() {
        let tool = MCPTool(id: "1", name: "test_tool", description: "A test tool")
        XCTAssertEqual(tool.name, "test_tool")
    }
}
