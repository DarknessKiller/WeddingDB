if (data.forcePasswordChange) {
        setAuth(loginAccessToken, loginRefreshToken, loginRole, loginName);
        goto('/change-password', { replaceState: true });
        return;
      }
