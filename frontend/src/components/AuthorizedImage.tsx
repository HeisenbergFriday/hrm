import React from 'react'
import { Image } from 'antd'
import { useAuthorizedFileUrl } from '../utils/authFileUrl'

interface AuthorizedImageProps extends Omit<React.ComponentProps<typeof Image>, 'src'> {
  src?: string
}

const AuthorizedImage: React.FC<AuthorizedImageProps> = ({ src, ...props }) => {
  const resolvedSrc = useAuthorizedFileUrl(src)
  return <Image {...props} src={resolvedSrc || undefined} />
}

export default AuthorizedImage
