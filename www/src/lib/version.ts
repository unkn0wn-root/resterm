import changelog from '../../../CHANGELOG.md?raw';

// The newest release is the first version heading in CHANGELOG.md.
export const version = /^#{2,3} \[(\d+\.\d+\.\d+)\]/m.exec(changelog)?.[1] ?? 'dev';
