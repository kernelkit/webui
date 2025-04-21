WebUI Portal Prototype for Infix
================================

This project uses Go+HTMX and Bootstrap to create a very basic WebUI
Portal for Infix, with proper login and session handling.

When started in developer mode authentication is user/pass: admin/admin,
by default however, proper PAM support is used.

Currently you need at least Go 1.21.  Remember to fetch all deps first:

```bash
$ go mod tidy
```

Then you can run the program:

```bash
$ make run
```

or developer/debug mode:

```bash
$ make dev
```


Screenshots
-----------

> [!NOTE]
> Out of date, but shows general design at least.

![](img/login-light.png)
![](img/login-dark.png)

![](img/portal-light.png)
![](img/portal-dark.png)
