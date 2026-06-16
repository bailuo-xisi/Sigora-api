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
 * Application-wide constants
 */

// System Configuration Defaults
const PUBLIC_SYSTEM_NAME =
  import.meta.env.VITE_PUBLIC_SYSTEM_NAME?.trim() || ''
export const DEFAULT_SYSTEM_NAME = PUBLIC_SYSTEM_NAME || 'New API'

const PUBLIC_LOGO = import.meta.env.VITE_PUBLIC_LOGO?.trim() || ''
export const DEFAULT_LOGO = PUBLIC_LOGO || '/logo.png'

export function resolvePublicSystemName(value?: string | null): string {
  const systemName = value?.trim()
  if (!systemName) {
    return DEFAULT_SYSTEM_NAME
  }
  if (PUBLIC_SYSTEM_NAME && systemName.toLowerCase() === 'new api') {
    return PUBLIC_SYSTEM_NAME
  }
  return systemName
}

// LocalStorage Keys
export const STORAGE_KEYS = {
  SYSTEM_NAME: 'system_name',
  LOGO: 'logo',
  FOOTER_HTML: 'footer_html',
} as const
