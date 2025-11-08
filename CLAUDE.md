# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Agent Instructions

For agent-specific instructions and workflows, see the [.claude/agents/](.claude/agents/) directory. Each agent has its own configuration file with specialized guidance.

## Project Overview

Tokensmith is an Envoy external authorizer for Envoy 1.27 that exchanges OIDC ID tokens for Kubernetes service accounts in one cluster for ID tokens for valid Kubernetes service accounts in another cluster. This enables secure cross-cluster authentication.

## Development Workflow

The preferred workflow for adding new features:

1. **Plan First**: Write a detailed plan to the [plan/](plan/) directory
2. **Human Review**: Wait for human review and approval of the plan
3. **Iterative Implementation**: Go through iteration cycles implementing the plan
4. **Commit at Each Step**: Create a git commit at each todo step completion
5. **Update Documentation**: Keep README.md and other docs current

## Project Structure

```
tokensmith/
├── .claude/agents/      # Agent-specific instructions
├── plan/                # Design and implementation plans
├── cmd/tokensmith/      # Main application entry point
├── internal/            # Internal packages
│   ├── authz/           # Envoy external authorization logic
│   ├── token/           # Token exchange and validation
│   └── config/          # Configuration management
├── api/envoy/           # Envoy external auth protobuf definitions
├── test/                # Tests and test data
└── deploy/kubernetes/   # Kubernetes deployment manifests
```

## Build and Test Commands

```bash
make build          # Build the tokensmith binary
make test           # Run all tests
make lint           # Run linters
make proto          # Generate protobuf code
make docker         # Build Docker image
```

## Code Conventions

- Follow standard Go conventions and idioms
- Use structured logging
- All public APIs must have godoc comments
- Maintain high test coverage for critical paths
- Use meaningful commit messages following conventional commits format
