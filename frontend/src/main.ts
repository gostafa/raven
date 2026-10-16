import { mount } from 'svelte';
import App from './App.svelte';
import { createRuntime } from './features/workspace/adapters';
const target = document.getElementById('app');
if (!target) throw new Error('Raven mount element is missing');
mount(App, { target, props: { backend: createRuntime() } });
