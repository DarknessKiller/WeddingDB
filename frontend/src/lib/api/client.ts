if (data.forcePasswordChange) {
						setAuth(data.accessToken, data.refreshToken, data.role ?? getAuth().role ?? '', data.name ?? getAuth().name ?? '');
						if (typeof window !== 'undefined') {
							window.location.href = '/change-password';
						}
						return res;
					}
