// Type declarations for bun:test
// Bun's built-in test runner types
declare module 'bun:test' {
  export function describe(name: string, fn: () => void): void
  export function test(name: string, fn: () => void | Promise<void>): void
  export function after(fn: () => void): void
  export function before(fn: () => void): void
  export function beforeEach(fn: () => void): void
  export function afterEach(fn: () => void): void
  export function expect(value: unknown): {
    toBe(expected: unknown): void
    toEqual(expected: unknown): void
    toBeNull(): void
    toBeDefined(): void
    toBeTruthy(): void
    toBeFalsy(): void
    toContain(item: unknown): void
    toThrow(): void
    not: {
      toBe(expected: unknown): void
      toEqual(expected: unknown): void
    }
  }
  export function mock(module: string, factory?: () => unknown): void
  export function spyOn(obj: object, method: string): { mock: { calls: unknown[] } }
  export type Mock = { calls: unknown[] }
}