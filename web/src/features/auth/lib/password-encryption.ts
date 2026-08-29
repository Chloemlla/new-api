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
import { t } from 'i18next'

import { api } from '@/lib/api'

interface PasswordEncryptionKey {
  kid: string
  public_key: string
  nonce: string
}

export interface EncryptedPassword {
  password_encrypted: string
  encryption_key_id: string
}

export async function encryptPassword(
  password: string
): Promise<EncryptedPassword> {
  let key: PasswordEncryptionKey
  try {
    key = await getPasswordEncryptionKey()
  } catch (error: unknown) {
    // Key-fetch failures mean the login service is unavailable, not that the
    // submitted credentials are wrong. Surface that distinction to the caller.
    throw new Error(t('Login service unavailable, please retry'), {
      cause: error,
    })
  }

  let ciphertext: string
  try {
    ciphertext = await rsaOaepEncrypt(
      JSON.stringify({ nonce: key.nonce, password }),
      key.public_key
    )
  } catch (error: unknown) {
    throw new Error(t('Login failed'), { cause: error })
  }

  return {
    password_encrypted: ciphertext,
    encryption_key_id: key.kid,
  }
}

async function getPasswordEncryptionKey(): Promise<PasswordEncryptionKey> {
  const response = await api.get<{
    success: boolean
    data?: PasswordEncryptionKey
  }>('/api/user/login/encryption-key', {
    skipErrorHandler: true,
    skipBusinessError: true,
    disableDuplicate: true,
  })
  const key = response.data?.data
  if (!response.data?.success || !key?.kid || !key.public_key || !key.nonce) {
    throw new Error('Password encryption key is unavailable')
  }
  return key
}

async function rsaOaepEncrypt(
  plaintext: string,
  publicKeyPEM: string
): Promise<string> {
  if (typeof globalThis.crypto?.subtle !== 'undefined') {
    try {
      const publicKey = await globalThis.crypto.subtle.importKey(
        'spki',
        pemToDER(publicKeyPEM),
        { name: 'RSA-OAEP', hash: 'SHA-256' },
        false,
        ['encrypt']
      )
      const ciphertext = await globalThis.crypto.subtle.encrypt(
        { name: 'RSA-OAEP' },
        publicKey,
        new TextEncoder().encode(plaintext)
      )
      return arrayBufferToBase64(ciphertext)
    } catch {
      // Older implementations may expose SubtleCrypto without supporting the
      // required RSA-OAEP parameters; the HTTP-compatible fallback handles it.
    }
  }

  // Web Crypto is restricted to secure contexts in browsers. Lazy-loading
  // forge keeps the normal HTTPS bundle small while supporting HTTP intranets.
  const forge = await import('node-forge')
  const publicKey = forge.pki.publicKeyFromPem(publicKeyPEM)
  const ciphertext = publicKey.encrypt(
    forge.util.encodeUtf8(plaintext),
    'RSA-OAEP',
    { md: forge.md.sha256.create() }
  )
  return forge.util.encode64(ciphertext)
}

function pemToDER(pem: string): ArrayBuffer {
  const body = pem
    .replace('-----BEGIN PUBLIC KEY-----', '')
    .replace('-----END PUBLIC KEY-----', '')
    .replaceAll(/\s+/g, '')
  const binary = atob(body)
  const bytes = new Uint8Array(binary.length)
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index)
  }
  return bytes.buffer
}

function arrayBufferToBase64(buffer: ArrayBuffer): string {
  const bytes = new Uint8Array(buffer)
  let binary = ''
  for (const byte of bytes) {
    binary += String.fromCharCode(byte)
  }
  return btoa(binary)
}
