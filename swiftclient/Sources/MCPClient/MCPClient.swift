import Foundation

public struct MCPConfig: Codable {
    public let serverURL: String
    public let apiKey: String?
    public let model: String
    
    public init(serverURL: String = "http://localhost:8080/mcp", apiKey: String? = nil, model: String = "llama3") {
        self.serverURL = serverURL
        self.apiKey = apiKey
        self.model = model
    }
}

public struct MCPTool: Codable, Identifiable {
    public let id: String
    public let name: String
    public let description: String
    
    enum CodingKeys: String, CodingKey {
        case id, name, description
    }
}

public struct MCPMessage: Codable {
    public let role: String
    public let content: String
    
    public init(role: String, content: String) {
        self.role = role
        self.content = content
    }
}

public struct MCPToolCall: Codable {
    public let id: String
    public let name: String
    public let arguments: [String: AnyCodable]
    
    public init(id: String, name: String, arguments: [String: AnyCodable]) {
        self.id = id
        self.name = name
        self.arguments = arguments
    }
}

public struct AnyCodable: Codable {
    public let value: Any
    
    public init(_ value: Any) {
        self.value = value
    }
    
    public init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        if let string = try? container.decode(String.self) {
            value = string
        } else if let int = try? container.decode(Int.self) {
            value = int
        } else if let double = try? container.decode(Double.self) {
            value = double
        } else if let bool = try? container.decode(Bool.self) {
            value = bool
        } else if let array = try? container.decode([AnyCodable].self) {
            value = array.map { $0.value }
        } else if let dict = try? container.decode([String: AnyCodable].self) {
            value = dict.mapValues { $0.value }
        } else {
            value = NSNull()
        }
    }
    
    public func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        if let string = value as? String {
            try container.encode(string)
        } else if let int = value as? Int {
            try container.encode(int)
        } else if let double = value as? Double {
            try container.encode(double)
        } else if let bool = value as? Bool {
            try container.encode(bool)
        } else if let array = value as? [Any] {
            try container.encode(array.map { AnyCodable($0) })
        } else if let dict = value as? [String: Any] {
            try container.encode(dict.mapValues { AnyCodable($0) })
        } else {
            try container.encodeNil()
        }
    }
}

public class MCPClient {
    private let config: MCPConfig
    private var messages: [MCPMessage] = []
    private let session: URLSession
    
    public init(config: MCPConfig = MCPConfig()) {
        self.config = config
        self.session = URLSession.shared
    }
    
    public func addMessage(_ message: MCPMessage) {
        messages.append(message)
    }
    
    public func send(prompt: String, tools: [MCPTool]? = nil) async throws -> String {
        messages.append(MCPMessage(role: "user", content: prompt))
        
        let request = try buildChatRequest(tools: tools)
        
        let (data, response) = try await session.data(for: request)
        
        guard let httpResponse = response as? HTTPURLResponse else {
            throw MCPError.invalidResponse
        }
        
        guard httpResponse.statusCode == 200 else {
            throw MCPError.serverError(statusCode: httpResponse.statusCode)
        }
        
        let chatResponse = try JSONDecoder().decode(ChatCompletionResponse.self, from: data)
        
        guard let choice = chatResponse.choices.first else {
            throw MCPError.noChoices
        }
        
        messages.append(MCPMessage(role: choice.message.role, content: choice.message.content))
        
        return choice.message.content
    }
    
    private func buildChatRequest(tools: [MCPTool]?) throws -> URLRequest {
        var request = URLRequest(url: URL(string: "\(config.serverURL)/chat")!)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        
        if let apiKey = config.apiKey {
            request.setValue("Bearer \(apiKey)", forHTTPHeaderField: "Authorization")
        }
        
        let chatRequest = ChatCompletionRequest(
            model: config.model,
            messages: messages.map { ChatMessage(role: $0.role, content: $0.content) },
            tools: tools?.map { ToolDefinition(function: FunctionDefinition(name: $0.name, description: $0.description)) },
            stream: false
        )
        
        request.httpBody = try JSONEncoder().encode(chatRequest)
        return request
    }
}

struct ChatCompletionRequest: Codable {
    let model: String
    let messages: [ChatMessage]
    let tools: [ToolDefinition]?
    let stream: Bool
}

struct ChatMessage: Codable {
    let role: String
    let content: String
}

struct ToolDefinition: Codable {
    let function: FunctionDefinition
}

struct FunctionDefinition: Codable {
    let name: String
    let description: String
}

struct ChatCompletionResponse: Codable {
    let choices: [Choice]
}

struct Choice: Codable {
    let message: ResponseMessage
}

struct ResponseMessage: Codable {
    let role: String
    let content: String
}

public enum MCPError: Error {
    case invalidResponse
    case serverError(statusCode: Int)
    case noChoices
}
