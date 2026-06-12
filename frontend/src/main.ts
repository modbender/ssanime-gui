import { mount } from 'svelte'
import '@fontsource-variable/plus-jakarta-sans'
import './app.css'
import App from './App.svelte'
import { APP_NAME } from '$lib/app'

document.title = APP_NAME

const app = mount(App, {
  target: document.getElementById('app')!,
})

export default app
