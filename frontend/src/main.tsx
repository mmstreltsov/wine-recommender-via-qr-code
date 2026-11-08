import { createRoot } from 'react-dom/client'
import { trackPageOpen } from './api/tracker'
import App from './App'
import './theme/base.css'
import './theme/aidabaran.css'

trackPageOpen()

const rootElement = document.getElementById('root')
if (!rootElement) throw new Error('root element not found')

createRoot(rootElement).render(<App />)
