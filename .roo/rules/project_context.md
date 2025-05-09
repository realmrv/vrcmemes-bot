---
description: 
globs: 
alwaysApply: true
---
# Rules for Roo Code AI in a Golang Project

## 1. Autocompletion According to Golang Style

- **Variable and Function Names**: Use `camelCase` style (e.g., `getUser`, `processData`) when suggesting names.
- **Type Names**: Use `PascalCase` style (e.g., `UserData`, `LoggerInterface`) for structs and interfaces.
- **Context Awareness**: Analyze the code context (data types, scope, dependencies) to provide autocompletion that fits the current situation.

## 2. Generating Templates for Error Handling

- **Error Checks**: For functions returning an `error`, consider inserting error handling that adds context or logs the error before returning. Example:

  ```go
  if err != nil {
      // Example: return fmt.Errorf("operation failed: %w", err) 
      // Or: log.Printf("Error during operation: %v", err); return err
      return err // Adjust based on context
  }
  ```

- **Error-Prone Operations**: When generating code for operations that might fail (e.g., file I/O, networking), always include explicit and informative error handling.

## 3. Automatic Code Documentation

- **Function Comments**: Suggest adding comments primarily for exported functions/methods or complex unexported ones, following this format:

  ```go
  // FunctionName describes the purpose of the function.
  func FunctionName() {
      // ...
  }
  ```

- **Explanatory Notes**: Suggest clarifying comments *only* for particularly complex or non-obvious code sections based on logic analysis.

## 4. Support for Navigation and Package Imports

- **Auto-Imports**: Suggest adding imports (e.g., `import "fmt"` for `fmt.Println`) when using external package functions or types.
- **Navigation**: Enable autocompletion for jumping to definitions of functions, types, and variables within the project.
- **Project Structure**: Respect the project's package layout (e.g., `main`, `models`, `utils`) when suggesting imports or navigation.

## 5. Assistance with Writing Tests

- **Test Template**: Use this standard format for test functions:

  ```go
  func TestFunctionName(t *testing.T) {
      // ...
  }
  ```

- **Table-Driven Tests**: Consider using table-driven tests for testing multiple scenarios efficiently.
- **Testing Tools**: Prefer standard library tools (`testing`, `net/http/httptest`) where possible. Offer autocompletion for tools like `t.Errorf` and standard mocking techniques.
- **Test Suggestions**: If a non-trivial function is added without a test, propose creating a matching test function.

## 6. Adherence to Golang Best Practices

- **No Global Variables**: Favor dependency injection via function parameters over global variables.
- **Minimal Interfaces**: Generate concise and explicit interface contracts.
- **Code Quality**: Suggest running `gofmt` for formatting and tools like `staticcheck` or `golangci-lint` for static analysis after code changes.

## 7. Context-Dependent Suggestions

- **Context Analysis**: Identify the current task (e.g., HTTP handling, database operations) and suggest relevant templates or functions (e.g., `http.Handler` for web servers).
- **Dependency Compatibility**: Propose code that aligns with the project's existing libraries and dependencies.

## 8. Flexibility and Adaptability

- **Project Settings**: Adapt to custom project configurations if rules are modified or disabled.
- **Feedback Learning**: Refine suggestions based on codebase updates and developer feedback.

## 9. Implementation and Behavior

- **Core Principle**: Prioritize simplicity and clarity, reflecting Golang's philosophy.
- **Focus on Execution**: Prioritize completing the requested task efficiently. Provide explanations mainly for non-obvious steps or when choices need justification, rather than detailing every routine action.
- **Error Handling**: Never omit error checks in generated code.
- **Tool Integration**: Collaborate with `gofmt` and static analysis tools (e.g., `staticcheck`, `golangci-lint`), recommending their use after modifications.
- **Version Awareness**: Stay updated with the latest Golang releases and adjust templates accordingly.

## 10. Language

- Always use English in generated files
- Always use English in comments
- Always use English in commit messages
- Always use English in README.md

## 11. Versions

- Go `1.24`
- github.com/mymmrac/telego `v1.0.2`

## 12. Adds

- Always use relative paths to files and folders.
- Keep dependencies reasonably up-to-date, considering compatibility and stability. Avoid forcing the absolute latest versions if unnecessary.
- Do not manually change the `go.sum` file; let Go tooling manage it.
- Specify database interaction patterns if applicable (e.g., preferred ORM/driver, migration strategy). [Placeholder: Add details if relevant]
- Respect `.gitignore` rules. Ignore files and folders from `.gitignore` files when scanning. Handle `.env` files carefully: they should typically be in `.gitignore` to avoid committing secrets. Ensure local setup instructions cover `.env` file creation if needed.
- **Avoid Large Files**: Avoid automatically reading or scanning excessively large files (e.g., over several thousand lines or >1MB) unless specifically required for the task and confirmed. Prefer targeted reading or searching within large files to conserve context.

## 13. Reporting and Verbosity

- **Concise Explanations**: Keep explanations for individual tool calls or actions brief, focusing on *why* a step is necessary only if it's not obvious. Avoid step-by-step narration of routine operations.
- **Summary Report**: At the conclusion of a multi-step task, provide a concise summary of the key actions taken and the final outcome.
