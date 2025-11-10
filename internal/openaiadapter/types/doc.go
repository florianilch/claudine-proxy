// Package api provides OpenAI API types for server-side request/response handling.
//
// This package uses oapi-codegen to generate types from OpenAPI specs rather than
// using openai-go SDK:
//
//  1. SERVER-SIDE vs CLIENT-SIDE: The openai-go SDK is designed for making outbound
//     API calls TO OpenAI. This adapter receives inbound requests FROM clients and
//     translates them TO Anthropic. The client-oriented design would add unnecessary
//     complexity for server-side JSON decoding.
//
//  2. FIELD PATTERNS: SDK uses param.Opt[T] or similar for optional fields, requiring
//     additional checks. Generated types use standard Go pointers (*string, *int),
//     which work naturally with standard library JSON unmarshaling via json.NewDecoder().
//
//  3. DEPENDENCIES: Generated types depend only on oapi-codegen/runtime. SDK types would
//     introduce additional internal packages.
//
//  4. STANDARD JSON: Generated types work with encoding/json directly. SDK types
//     would require custom marshaling logic.
package types
