// TEMPORARY placeholder — will be overwritten by make gen-types
//
// Ručně napsáno podle docs/SPEC.md §3 a §4.1 ve tvaru výstupu openapi-typescript,
// dokud backend neumí vygenerovat OpenAPI. Aplikace odkazuje na typy jen přes
// `paths` (viz src/api/types.ts), takže názvy v `components` se smí po generování změnit.

export interface paths {
  "/api/health": {
    parameters: { query?: never; header?: never; path?: never; cookie?: never };
    get: operations["health"];
    put?: never;
    post?: never;
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/auth/status": {
    parameters: { query?: never; header?: never; path?: never; cookie?: never };
    get: operations["auth-status"];
    put?: never;
    post?: never;
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/auth/register": {
    parameters: { query?: never; header?: never; path?: never; cookie?: never };
    get?: never;
    put?: never;
    post: operations["auth-register"];
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/auth/login": {
    parameters: { query?: never; header?: never; path?: never; cookie?: never };
    get?: never;
    put?: never;
    post: operations["auth-login"];
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/auth/logout": {
    parameters: { query?: never; header?: never; path?: never; cookie?: never };
    get?: never;
    put?: never;
    post: operations["auth-logout"];
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/auth/me": {
    parameters: { query?: never; header?: never; path?: never; cookie?: never };
    get: operations["auth-me"];
    put?: never;
    post?: never;
    delete?: never;
    options?: never;
    head?: never;
    patch: operations["auth-update-me"];
    trace?: never;
  };
  "/api/auth/tokens": {
    parameters: { query?: never; header?: never; path?: never; cookie?: never };
    get: operations["auth-list-tokens"];
    put?: never;
    post: operations["auth-create-token"];
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/auth/tokens/{id}": {
    parameters: { query?: never; header?: never; path?: never; cookie?: never };
    get?: never;
    put?: never;
    post?: never;
    delete: operations["auth-delete-token"];
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/accounts": {
    parameters: { query?: never; header?: never; path?: never; cookie?: never };
    get: operations["accounts-list"];
    put?: never;
    post: operations["accounts-create"];
    delete?: never;
    options?: never;
    head?: never;
    patch?: never;
    trace?: never;
  };
  "/api/accounts/{slug}": {
    parameters: { query?: never; header?: never; path?: never; cookie?: never };
    get: operations["accounts-get"];
    put?: never;
    post?: never;
    delete?: never;
    options?: never;
    head?: never;
    patch: operations["accounts-update"];
    trace?: never;
  };
}
export type webhooks = Record<string, never>;
export interface components {
  schemas: {
    Health: {
      readonly $schema?: string;
      status: string;
    };
    AuthStatus: {
      readonly $schema?: string;
      signup_allowed: boolean;
      has_users: boolean;
    };
    RegisterInput: {
      readonly $schema?: string;
      /** Format: email */
      email: string;
      name: string;
      password: string;
      account_name: string;
    };
    LoginInput: {
      readonly $schema?: string;
      /** Format: email */
      email: string;
      password: string;
    };
    User: {
      /** Format: int64 */
      id: number;
      email: string;
      name: string;
    };
    AccountMembership: {
      slug: string;
      name: string;
      /** @enum {string} */
      role: "owner" | "member";
    };
    Me: {
      readonly $schema?: string;
      user: components["schemas"]["User"];
      accounts: components["schemas"]["AccountMembership"][] | null;
    };
    UpdateMeInput: {
      readonly $schema?: string;
      name?: string;
      current_password?: string;
      new_password?: string;
    };
    APIToken: {
      /** Format: int64 */
      id: number;
      name: string;
      prefix: string;
      /** Format: date-time */
      last_used_at?: string | null;
      /** Format: date-time */
      created_at: string;
    };
    APITokenList: {
      readonly $schema?: string;
      items: components["schemas"]["APIToken"][] | null;
    };
    CreateTokenInput: {
      readonly $schema?: string;
      name: string;
    };
    APITokenCreated: {
      readonly $schema?: string;
      /** Format: int64 */
      id: number;
      name: string;
      prefix: string;
      /** @description Plaintext token — vrácen pouze jednou při vytvoření. */
      token: string;
      /** Format: date-time */
      created_at: string;
    };
    AccountList: {
      readonly $schema?: string;
      items: components["schemas"]["AccountMembership"][] | null;
    };
    CreateAccountInput: {
      readonly $schema?: string;
      name: string;
    };
    Account: {
      readonly $schema?: string;
      slug: string;
      name: string;
      registration_no: string;
      vat_no: string;
      street: string;
      city: string;
      zip: string;
      country: string;
      email: string;
      phone: string;
      web: string;
      /** @enum {string} */
      vat_mode: "non_vat_payer" | "vat_payer" | "identified_person";
      registered_by: string;
      default_currency: string;
      /** Format: int64 */
      default_due_days: number;
      default_payment_method: string;
      /** @enum {string} */
      default_language: "cs" | "en";
      default_note: string;
      default_footer_note: string;
      round_total: boolean;
      /** Format: int32 */
      default_vat_rate_bps: number;
      /** Format: date-time */
      created_at: string;
      /** Format: date-time */
      updated_at: string;
    };
    UpdateAccountInput: {
      readonly $schema?: string;
      name?: string;
      registration_no?: string;
      vat_no?: string;
      street?: string;
      city?: string;
      zip?: string;
      country?: string;
      email?: string;
      phone?: string;
      web?: string;
      /** @enum {string} */
      vat_mode?: "non_vat_payer" | "vat_payer" | "identified_person";
      registered_by?: string;
      default_currency?: string;
      /** Format: int64 */
      default_due_days?: number;
      default_payment_method?: string;
      /** @enum {string} */
      default_language?: "cs" | "en";
      default_note?: string;
      default_footer_note?: string;
      round_total?: boolean;
      /** Format: int32 */
      default_vat_rate_bps?: number;
    };
    ErrorDetail: {
      location?: string;
      message?: string;
      value?: unknown;
    };
    ErrorModel: {
      readonly $schema?: string;
      detail?: string;
      errors?: components["schemas"]["ErrorDetail"][] | null;
      /** Format: uri */
      instance?: string;
      /** Format: int64 */
      status?: number;
      title?: string;
      /**
       * Format: uri
       * @default about:blank
       */
      type: string;
    };
  };
  responses: never;
  parameters: never;
  requestBodies: never;
  headers: never;
  pathItems: never;
}
export type $defs = Record<string, never>;

type ErrorResponse = {
  headers: { [name: string]: unknown };
  content: { "application/problem+json": components["schemas"]["ErrorModel"] };
};
type NoContent = { headers: { [name: string]: unknown }; content?: never };
type Json<T> = { headers: { [name: string]: unknown }; content: { "application/json": T } };
type NoParams = { query?: never; header?: never; path?: never; cookie?: never };

export interface operations {
  health: {
    parameters: NoParams;
    requestBody?: never;
    responses: { 200: Json<components["schemas"]["Health"]>; default: ErrorResponse };
  };
  "auth-status": {
    parameters: NoParams;
    requestBody?: never;
    responses: { 200: Json<components["schemas"]["AuthStatus"]>; default: ErrorResponse };
  };
  "auth-register": {
    parameters: NoParams;
    requestBody: { content: { "application/json": components["schemas"]["RegisterInput"] } };
    responses: { 200: Json<components["schemas"]["Me"]>; default: ErrorResponse };
  };
  "auth-login": {
    parameters: NoParams;
    requestBody: { content: { "application/json": components["schemas"]["LoginInput"] } };
    responses: { 200: Json<components["schemas"]["Me"]>; default: ErrorResponse };
  };
  "auth-logout": {
    parameters: NoParams;
    requestBody?: never;
    responses: { 204: NoContent; default: ErrorResponse };
  };
  "auth-me": {
    parameters: NoParams;
    requestBody?: never;
    responses: { 200: Json<components["schemas"]["Me"]>; default: ErrorResponse };
  };
  "auth-update-me": {
    parameters: NoParams;
    requestBody: { content: { "application/json": components["schemas"]["UpdateMeInput"] } };
    responses: { 200: Json<components["schemas"]["Me"]>; default: ErrorResponse };
  };
  "auth-list-tokens": {
    parameters: NoParams;
    requestBody?: never;
    responses: { 200: Json<components["schemas"]["APITokenList"]>; default: ErrorResponse };
  };
  "auth-create-token": {
    parameters: NoParams;
    requestBody: { content: { "application/json": components["schemas"]["CreateTokenInput"] } };
    responses: { 200: Json<components["schemas"]["APITokenCreated"]>; default: ErrorResponse };
  };
  "auth-delete-token": {
    parameters: { query?: never; header?: never; path: { id: number }; cookie?: never };
    requestBody?: never;
    responses: { 204: NoContent; default: ErrorResponse };
  };
  "accounts-list": {
    parameters: NoParams;
    requestBody?: never;
    responses: { 200: Json<components["schemas"]["AccountList"]>; default: ErrorResponse };
  };
  "accounts-create": {
    parameters: NoParams;
    requestBody: { content: { "application/json": components["schemas"]["CreateAccountInput"] } };
    responses: { 200: Json<components["schemas"]["Account"]>; default: ErrorResponse };
  };
  "accounts-get": {
    parameters: { query?: never; header?: never; path: { slug: string }; cookie?: never };
    requestBody?: never;
    responses: { 200: Json<components["schemas"]["Account"]>; default: ErrorResponse };
  };
  "accounts-update": {
    parameters: { query?: never; header?: never; path: { slug: string }; cookie?: never };
    requestBody: { content: { "application/json": components["schemas"]["UpdateAccountInput"] } };
    responses: { 200: Json<components["schemas"]["Account"]>; default: ErrorResponse };
  };
}
