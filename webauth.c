/* SPDX-License-Identifier: ISC */

#include <security/pam_appl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

/* Custom PAM conversation function that reads the password from stdin */
int custom_pam_conv(int num, const struct pam_message **msg, struct pam_response **resp, void *_)
{
	struct pam_response *response;
	char password[384];

	if (num != 1 || msg[0]->msg_style != PAM_PROMPT_ECHO_OFF) {
		fprintf(stderr, "Unsupported operation or configuration\n");
		return PAM_CONV_ERR;
	}

	response = (struct pam_response *)calloc(1, sizeof(struct pam_response));
	if (!response)
		return PAM_BUF_ERR;

	if (fgets(password, sizeof(password), stdin) == NULL) {
		fprintf(stderr, "Failed to read password\n");
		free(response);
		return PAM_CONV_ERR;
	}
	password[strcspn(password, "\n")] = 0;

	response[0].resp = strdup(password);
	response[0].resp_retcode = 0;
	*resp = response;

	return PAM_SUCCESS;
}

int main(int argc, char *argv[])
{
	static struct pam_conv conv = {
		custom_pam_conv,
		NULL
	};
	const char *user = argv[1];
	pam_handle_t *pamh = NULL;
	int rc;
    
	if (argc != 2) {
		fprintf(stderr, "Usage: %s <username>\n\nPassword is expected on stdin.\n", argv[0]);
		return 1;
	}

	rc = pam_start("webauth", user, &conv, &pamh);
	if (rc == PAM_SUCCESS)
		rc = pam_authenticate(pamh, 0);

	if (pam_end(pamh, rc) != PAM_SUCCESS) {
		fprintf(stderr, "Failed to release PAM resources\n");
		pamh = NULL;
		return 1;
	}

	return rc == PAM_SUCCESS ? 0 : 1;
}
