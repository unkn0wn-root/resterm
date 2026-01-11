package curl

func ParseCommandsInfo(command string) ([]Res, error) {
	tok, err := splitTokens(command)
	if err != nil {
		return nil, err
	}
	cmd, err := parseCmd(tok)
	if err != nil {
		return nil, err
	}
	return normCmdRes(cmd)
}
