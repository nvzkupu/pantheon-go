Apply the @themis skill. Write tests for the specified code.

1. Read the implementation files and understand the code under test
2. Identify critical paths and edge cases
3. Check existing test patterns in the project — match conventions
4. Write tests: table-driven where applicable, clear names, Arrange-Act-Assert
5. Run the tests — confirm they pass
6. Verify tests fail when the target behavior is removed (mental mutation test)

Prioritize: critical paths first, edge cases second, happy paths last (they're usually already covered).
