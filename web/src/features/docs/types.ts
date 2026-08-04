/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
/**
 * Minimal TypeScript model for the OpenAPI 3.x documents served by the
 * backend at /api/docs. Only the parts the documentation page renders are
 * modeled; the rest flows through as `any`.
 */

export type OpenAPISchema = {
  type?: string
  format?: string
  title?: string
  description?: string
  enum?: unknown[]
  default?: unknown
  example?: unknown
  nullable?: boolean
  deprecated?: boolean
  readOnly?: boolean
  required?: string[]
  properties?: Record<string, OpenAPISchema>
  items?: OpenAPISchema
  additionalProperties?: OpenAPISchema | boolean
  oneOf?: OpenAPISchema[]
  anyOf?: OpenAPISchema[]
  allOf?: OpenAPISchema[]
  $ref?: string
  minimum?: number
  maximum?: number
  minItems?: number
  maxItems?: number
  pattern?: string
  // Arbitrary extension fields (media/example shapes etc.)
  [key: string]: unknown
}

export type OpenAPIParameter = {
  name: string
  in: string
  required?: boolean
  description?: string
  deprecated?: boolean
  schema?: OpenAPISchema
  example?: unknown
}

export type OpenAPIMediaContent = {
  schema?: OpenAPISchema
  example?: unknown
  examples?: Record<string, unknown>
}

export type OpenAPIOperation = {
  summary?: string
  description?: string
  operationId?: string
  tags?: string[]
  deprecated?: boolean
  parameters?: OpenAPIParameter[]
  requestBody?: {
    required?: boolean
    description?: string
    content?: Record<string, OpenAPIMediaContent>
  }
  responses?: Record<string, OpenAPIResponse>
  security?: Record<string, unknown[]>[]
}

export type OpenAPIResponse = {
  description?: string
  content?: Record<string, OpenAPIMediaContent>
  headers?: Record<string, unknown>
  $ref?: string
}

export type OpenAPISecurityScheme = {
  type?: string
  scheme?: string
  bearerFormat?: string
  in?: string
  name?: string
  description?: string
  group?: { id: string | number }[]
}

export type OpenAPISpec = {
  openapi: string
  info: {
    title?: string
    description?: string
    version?: string
    termsOfService?: string
  }
  servers?: { url: string; description?: string }[]
  tags?: { name: string; description?: string }[]
  paths: Record<string, Record<string, OpenAPIOperation>>
  components?: {
    schemas?: Record<string, OpenAPISchema>
    responses?: Record<string, OpenAPIResponse>
    securitySchemes?: Record<string, OpenAPISecurityScheme>
  }
  security?: Record<string, unknown[]>[]
}
