package console

import "os/exec"

func configureProcess(cmd *exec.Cmd) {}
func killProcess(cmd *exec.Cmd) {
	if cmd.Process != nil {
		cmd.Process.Kill()
	}
}
