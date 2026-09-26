{
    This file is part of the Free Pascal run time library.
    Copyright (c) 2007 by Michael Van Canneyt, member of the
    Free Pascal development team

    MySQL 5.1 Data Dictionary Engine Implementation.

    See the file COPYING.FPC, included in this distribution,
    for details about the copyright.

    This program is distributed in the hope that it will be useful,
    but WITHOUT ANY WARRANTY; without even the implied warranty of
    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.

 **********************************************************************}
{$IFNDEF FPC_DOTTEDUNITS}
unit FPDDMySQL51;
{$ENDIF FPC_DOTTEDUNITS}

{$mode objfpc}{$H+}

interface

{$IFDEF FPC_DOTTEDUNITS}
uses
  System.Classes, System.SysUtils, Data.Sqldb, Data.Dict.Base, Data.Dict.Sqldb;
{$ELSE FPC_DOTTEDUNITS}
uses
  Classes, SysUtils, sqldb, fpdatadict, fpddsqldb;
{$ENDIF FPC_DOTTEDUNITS}

Type
  { TSQLDBMySql51DDEngine }

  TSQLDBMySql51DDEngine = Class(TSQLDBDDEngine)
  Protected
    Function CreateConnection(AConnectString  : String) : TSQLConnection; override;
  Public
    Class function Description : string; override;
    Class function DBType : String; override;
  end;

Procedure RegisterMySQL51DDEngine;
Procedure UnRegisterMySQL51DDEngine;

implementation

{$IFDEF FPC_DOTTEDUNITS}
uses Data.SqlDb.Mysql51;
{$ELSE FPC_DOTTEDUNITS}
uses mysql51conn;
{$ENDIF FPC_DOTTEDUNITS}

Procedure RegisterMySQL51DDEngine;

begin
  RegisterDictionaryEngine(TSQLDBMySQL51DDEngine);
end;

Procedure UnRegisterMySQL51DDEngine;

begin
  UnRegisterDictionaryEngine(TSQLDBMySQL51DDEngine);
end;

{ TSQLDBMySql51DDEngine }

function TSQLDBMySql51DDEngine.CreateConnection(AConnectString: String
  ): TSQLConnection;
begin
  Result:={$IFDEF FPC_DOTTEDUNITS}Data.SqlDb.Mysql51{$ELSE}Mysql51conn{$ENDIF}.TMySQL51Connection.Create(Self);
end;

class function TSQLDBMySql51DDEngine.Description: string;
begin
  Result:='Mysql 5.1 connection using SQLDB';
end;

class function TSQLDBMySql51DDEngine.DBType: String;
begin
  Result:='MySQL 5.1';
end;

end.

