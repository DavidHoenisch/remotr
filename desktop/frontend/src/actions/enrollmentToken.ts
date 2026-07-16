export interface EnrollmentTokenRequest {
  fleet: string;
  ttlSeconds: number;
}

export interface EnrollmentTokenResult {
  expiresAt: string;
  fleet: string;
  token: string;
}
