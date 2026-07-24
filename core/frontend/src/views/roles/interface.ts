export interface RoleParams {
	page: number
	page_size: number
	name: string
}

export interface Permission {
	id: number
	name: string
	description: string
	module: string
	action: string
	resource: string
	status: number
}

export interface Role {
	id: number
	name: string
	description: string
	status: number
	create_time: number
}
