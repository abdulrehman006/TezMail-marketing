import { createDiscreteApi } from 'naive-ui'

const { message } = createDiscreteApi(['message'], {
	configProviderProps: {
		themeOverrides: {
			Message: {
				colorSuccess: '#20a53a',
				colorError: '#ef0808',
				colorWarning: '#f0ad4e',
				colorInfo: '#2080f0',
			},
		},
	},
})

export default message
