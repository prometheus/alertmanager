module Http.Internal exposing
  ( Request(..)
  , RawRequest
  , Expect
  , Body(..)
  , Header(..)
  , map
  )


import Elm.Kernel.Http



type Request a = Request (RawRequest a)


type alias RawRequest a =
    { method : String
    , headers : List Header
    , url : String
    , body : Body
    , expect : Expect a
    , timeout : Maybe Float
    , withCredentials : Bool
    }


type Expect a = Expect


type Body
  = EmptyBody
  | StringBody String String
  | FormDataBody ()



type Header = Header String String


map : (a -> b) -> RawRequest a -> RawRequest b
map func { method, headers, url, body, expect, timeout, withCredentials } =
  { method = method
  , headers = headers
  , url = url
  , body = body
  , expect = Elm.Kernel.Http.mapExpect func expect
  , timeout = timeout
  , withCredentials = withCredentials
  }


type Xhr = Xhr


isStringBody : Body -> Bool
isStringBody body =
  case body of
    StringBody _ _ ->
      True

    _ ->
      False
