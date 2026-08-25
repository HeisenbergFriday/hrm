export function unwrapApiData<T>(response: any, fallback: T): T {
  return (response?.data || response || fallback) as T
}

export function unwrapApiItems<T>(response: any): T[] {
  const data = unwrapApiData<any>(response, {})
  return data?.items || []
}

export function unwrapApiList<T>(response: any, key: string): T[] {
  const data = unwrapApiData<any>(response, {})
  return data?.[key] || data?.items || []
}
