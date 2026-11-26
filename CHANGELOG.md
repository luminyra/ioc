# Changelog

All notable changes to this project are documented in this file. The format
is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Dependency injection container with generic `Provide`, `Supply` and `Resolve` methods, typed `Key[T]` names, health checks and ordered shutdown.
- Dependency cycles across concurrent resolutions, such as one goroutine resolving `a -> b` while another resolves `b -> a`, are reported with `ErrCycle` instead of deadlocking.
- Benchmarks (`make bench`).
- A Makefile with the development tasks (`make help`).
- Requires Go 1.27 or later.
