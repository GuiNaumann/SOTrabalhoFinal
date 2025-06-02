--TODO: RODAR o monitor
go build -o central.exe central.go
.\central.exe

--TODO: vai rodar na porta que estiver livre
 http://localhost:8080/agents

--TODO:monitorar
 http://localhost:8080/

--TODO:#################################################################################################################
--TODO:#################################################################################################################
--TODO:#################################################################################################################
--TODO:################################## monitorar ####################################################################
--TODO:#################################################################################################################
--TODO:#################################################################################################################
--TODO:#################################################################################################################

 cd C:\dev\SOTrabalhoFinal\agent

--TODO:monitorar 1) Compile o agente para Windows, gerando agent.exe:
go build -o agent.exe agent.go

--TODO:monitorar 2) Aponte a variável de ambiente para o central:
$env:CENTRAL_URL = "http://localhost:8080/register"

--TODO:monitorar (Opcional) Se quiser mudar a porta RPC do agente:
--TODO:monitorar $env:AGENT_RPC_PORT = "9001"

--TODO:monitorar 3) Execute o agente:
.\agent.exe



