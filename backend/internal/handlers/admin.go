if body.Password == "" {
		return nil, fuego.BadRequestError{Title: "Password is required"}
