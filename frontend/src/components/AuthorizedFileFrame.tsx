import React from 'react'
import { useAuthorizedFileUrl } from '../utils/authFileUrl'

interface AuthorizedFileFrameProps extends React.IframeHTMLAttributes<HTMLIFrameElement> {
  src?: string
}

const AuthorizedFileFrame: React.FC<AuthorizedFileFrameProps> = ({ src, ...props }) => {
  const resolvedSrc = useAuthorizedFileUrl(src)
  return <iframe {...props} src={resolvedSrc || undefined} />
}

export default AuthorizedFileFrame
