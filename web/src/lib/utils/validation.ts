const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

export function validateEmail(v: string): string {
  if (!v) return 'Email is required.';
  if (!EMAIL_RE.test(v)) return 'Enter a valid email address.';
  return '';
}

export function validateUsername(v: string): string {
  if (!v) return 'Username is required.';
  if (v.length < 4) return 'Username must be at least 4 characters.';
  return '';
}

export function validatePassword(v: string): string {
  if (!v) return 'Password is required.';
  if (v.length < 8) return 'Must be at least 8 characters.';
  if (!/[A-Z]/.test(v)) return 'Must contain at least one uppercase letter.';
  if (!/[a-z]/.test(v)) return 'Must contain at least one lowercase letter.';
  if (!/[0-9]/.test(v)) return 'Must contain at least one number.';
  if (!/[^A-Za-z0-9]/.test(v)) return 'Must contain at least one special character.';
  return '';
}
