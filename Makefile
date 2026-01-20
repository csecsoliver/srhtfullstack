repos = core.sr.ht meta.sr.ht todo.sr.ht scm.sr.ht git.sr.ht man.sr.ht paste.sr.ht hub.sr.ht lists.sr.ht builds.sr.ht

.PHONY: init
.ONESHELL:
init: git-sshd/ssh_host_rsa_key git-sshd/ssh_host_ed25519_key
	@
	chmod 600 git-sshd/ssh_host_rsa_key git-sshd/ssh_host_ed25519_key
	for repo in ${repos}; do
		[ -e $$repo ] || git clone --recurse-submodules https://git.sr.ht/~sircmpwn/$$repo
		git -C $$repo config sendemail.to '~sircmpwn/sr.ht-dev@lists.sr.ht'
		git -C $$repo config format.subjectPrefix "PATCH $$repo"
	done

.PHONY: pull
.ONESHELL:
pull:
	@
	for repo in ${repos}; do
		git -C $$repo pull
	done

.PRONY: status
.ONESHELL:
status:
	@
	for repo in ${repos}; do
		echo === $$repo ===
		git -C $$repo status -s
	done

git-sshd/ssh_host_rsa_key:
	ssh-keygen -f git-sshd/ssh_host_rsa_key -N '' -C 'git-ssh' -t rsa -b 4096 && chmod 600 git-sshd/ssh_host_rsa_key
git-sshd/ssh_host_ed25519_key:
	ssh-keygen -f git-sshd/ssh_host_ed25519_key -N '' -C 'git-ssh' -t ed25519 && chmod 600 git-sshd/ssh_host_ed25519_key
