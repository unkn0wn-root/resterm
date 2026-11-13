export function throwIfAny(errors: readonly string[], message: string): void {
  if (errors.length) {
    throw new Error(`${message}:\n  ${errors.join('\n  ')}`);
  }
}
