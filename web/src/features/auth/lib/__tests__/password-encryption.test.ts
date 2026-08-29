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
import { webcrypto } from 'node:crypto'
import { beforeEach, describe, expect, test, vi } from 'vitest'

import { encryptPassword } from '../password-encryption'

const { get } = vi.hoisted(() => ({
  get: vi.fn(),
}))

vi.mock('@/lib/api', () => ({
  api: {
    get,
  },
}))

function spkiToPem(der: ArrayBuffer): string {
  const b64 = Buffer.from(der).toString('base64')
  const lines = b64.match(/.{1,64}/g) ?? []
  return `-----BEGIN PUBLIC KEY-----\n${lines.join('\n')}\n-----END PUBLIC KEY-----`
}

describe('encryptPassword', () => {
  beforeEach(() => {
    get.mockReset()
  })

  test('encrypts the server-issued nonce and password into a decryptable envelope', async () => {
    const keyPair = await webcrypto.subtle.generateKey(
      {
        name: 'RSA-OAEP',
        modulusLength: 2048,
        publicExponent: new Uint8Array([1, 0, 1]),
        hash: 'SHA-256',
      },
      true,
      ['encrypt', 'decrypt']
    )
    const spki = await webcrypto.subtle.exportKey('spki', keyPair.publicKey)
    const nonce = 'a'.repeat(64)
    get.mockResolvedValue({
      data: {
        success: true,
        data: { kid: 'kid-1', public_key: spkiToPem(spki), nonce },
      },
    })

    const result = await encryptPassword('correct horse battery staple')

    expect(result.encryption_key_id).toBe('kid-1')

    const privateKey = await webcrypto.subtle.importKey(
      'pkcs8',
      await webcrypto.subtle.exportKey('pkcs8', keyPair.privateKey),
      { name: 'RSA-OAEP', hash: 'SHA-256' },
      false,
      ['decrypt']
    )
    const plaintext = await webcrypto.subtle.decrypt(
      { name: 'RSA-OAEP' },
      privateKey,
      Buffer.from(result.password_encrypted, 'base64')
    )
    expect(JSON.parse(Buffer.from(plaintext).toString('utf-8'))).toEqual({
      nonce,
      password: 'correct horse battery staple',
    })
  })

  test('reports the login service as unavailable when the key cannot be fetched', async () => {
    get.mockRejectedValue(new Error('network down'))

    await expect(encryptPassword('password')).rejects.toThrow(
      'Login service unavailable, please retry'
    )
    expect(get).toHaveBeenCalledWith(
      '/api/user/login/encryption-key',
      expect.anything()
    )
  })

  test('reports the login service as unavailable when the server fails to issue a key', async () => {
    get.mockResolvedValue({ data: { success: false, message: 'database error' } })

    await expect(encryptPassword('password')).rejects.toThrow(
      'Login service unavailable, please retry'
    )
  })

  test('reports a login failure when the password cannot be encrypted', async () => {
    get.mockResolvedValue({
      data: {
        success: true,
        data: {
          kid: 'kid-1',
          public_key: 'not a pem',
          nonce: 'b'.repeat(64),
        },
      },
    })

    await expect(encryptPassword('password')).rejects.toThrow('Login failed')
  })
})
