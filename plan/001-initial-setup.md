# Plan 001: Initial Tokensmith Project Setup

**Status**: Complete

## Overview
Initialize tokensmith as a Go-based Envoy external authorizer for exchanging OIDC ID tokens between Kubernetes clusters.

## Project Purpose
Tokensmith exchanges OIDC ID tokens for Kubernetes service accounts in one cluster for ID tokens for valid Kubernetes service accounts in another cluster. This enables secure cross-cluster authentication for Istio 1.27.

## Project Structure
The project will follow a similar structure to the holos project:

```
tokensmith/
├── .claude/
│   └── agents/          # Agent-specific instructions
├── plan/                # Design and implementation plans
├── cmd/
│   └── tokensmith/      # Main application entry point
├── internal/
│   ├── authz/           # Envoy external authorization logic
│   ├── token/           # Token exchange and validation
│   └── config/          # Configuration management
├── api/
│   └── envoy/           # Envoy external auth protobuf definitions
├── pkg/                 # Public packages (if needed)
├── test/
│   ├── integration/     # Integration tests
│   └── testdata/        # Test fixtures and data
├── deploy/
│   └── kubernetes/      # Kubernetes deployment manifests
├── go.mod
├── go.sum
├── Makefile
├── .gitignore
├── README.md
├── CLAUDE.md            # Claude Code instructions
└── LICENSE
```

## Implementation Steps

### 1. Project Initialization
- Initialize Go module: `go mod init github.com/holos-run/tokensmith`
- Create directory structure
- Set up .gitignore for Go projects
- Create basic Makefile with common targets (build, test, lint)

### 2. Documentation
- Update CLAUDE.md with:
  - Pointer to .claude/agents/ directory for agent instructions
  - Project architecture and conventions
  - Development workflow
- Create comprehensive README.md explaining:
  - Project purpose (OIDC token exchange for cross-cluster auth)
  - How it works with Istio 1.27
  - Build and deployment instructions

### 3. Core Dependencies
- Envoy external auth API (envoy-go-control-plane)
- gRPC and protobuf
- Kubernetes client-go (for service account token validation)
- OIDC libraries (go-oidc or similar)
- Configuration management (viper or similar)

### 4. Basic Project Files
- Create LICENSE file (check holos for license type)
- Set up GitHub Actions or CI configuration
- Create CODEOWNERS if needed

## Development Workflow
1. Write plan to `plan/` directory
2. Human reviews plan
3. Implement plan iteratively
4. At each todo step, create git commit
5. Update README.md with progress

## Next Steps After Approval
1. Initialize Go module
2. Create directory structure
3. Set up basic project files
4. Create README.md with project description
5. Update CLAUDE.md with development guidance
6. Initial git commit

## Questions for Review
- Should we use a specific OIDC provider (e.g., Dex, Keycloak)?
- What logging framework should we use?
- Any specific testing frameworks preferred?
- License type (Apache 2.0, MIT, etc.)?
