export interface LoginRequest {
  email: string
  password: string
}

export interface AuthTokenResponse {
  strategy: string
  subject: string
  accessToken: string
}
