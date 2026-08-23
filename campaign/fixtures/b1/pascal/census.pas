program Census;
type
  TStatus = (stPass, stFallback, stSkip);
var
  s: TStatus;
begin
  for s := Low(TStatus) to High(TStatus) do
    WriteLn(Ord(s));
end.
