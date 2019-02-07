{-# LANGUAGE OverloadedStrings #-}

import Control.Monad (liftM, liftM2, liftM3)
import Data.Binary (Binary, encodeFile, get, put, getWord8, putWord8)
import Data.Map (Map, fromList)
import Data.Text (Text)
import Data.Word (Word16)


data Name =
  Name
    { _author :: !Text
    , _project :: !Text
    }
    deriving (Eq, Ord)


data Version =
    Version
        { _major :: {-# UNPACK #-} !Word16
        , _minor :: {-# UNPACK #-} !Word16
        , _patch :: {-# UNPACK #-} !Word16
        }
    deriving (Eq, Ord)


data PackageRegistry =
    PackageRegistry Int (Map Name [Version])


instance Binary PackageRegistry where
    get = liftM2 PackageRegistry get get
    put (PackageRegistry a b) = put a >> put b


instance Binary Name where
    get =
      liftM2 Name get get

    put (Name author project) =
      do  put author
          put project


instance Binary Version where
  get =
    do  word <- getWord8
        if word == 0
          then liftM3 Version get get get
          else
            do  minor <- liftM fromIntegral getWord8
                patch <- liftM fromIntegral getWord8
                return (Version (fromIntegral word) minor patch)

  put (Version major minor patch) =
    if major < 256 && minor < 256 && patch < 256 then
      do  putWord8 (fromIntegral major)
          putWord8 (fromIntegral minor)
          putWord8 (fromIntegral patch)
    else
      do  putWord8 0
          put major
          put minor
          put patch


main :: IO ()
main =
    encodeFile "./versions.dat" $
        PackageRegistry 0 $
            fromList
                [ ( Name "elm" "core", [ Version 1 0 0 ] )
                , ( Name "elm" "browser", [ Version 1 0 0] )
                , ( Name "elm" "html", [ Version 1 0 0] )
                , ( Name "elm" "random", [ Version 1 0 0] )
                , ( Name "elm" "json", [ Version 1 0 0] )
                , ( Name "elm" "time", [ Version 1 0 0] )
                , ( Name "elm" "url", [ Version 1 0 0] )
                , ( Name "elm" "virtual-dom", [ Version 1 0 0] )
                , ( Name "eeue56" "elm-lazy-list", [ Version 2 0 0] )
                , ( Name "eeue56" "elm-lazy", [ Version 1 0 1] )
                , ( Name "eeue56" "elm-shrink", [ Version 2 0 0] )
                , ( Name "jinjor" "elm-diff", [ Version 1 0 5] )
                , ( Name "elm-explorations" "test", [ Version 1 0 0] )
                ]
