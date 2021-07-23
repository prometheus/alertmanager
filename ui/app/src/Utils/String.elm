module Utils.String exposing (capitalizeFirst, linkify)

import Char
import String


capitalizeFirst : String -> String
capitalizeFirst string =
    case String.uncons string of
        Nothing ->
            string

        Just ( char, rest ) ->
            String.cons (Char.toUpper char) rest


linkify : String -> List (Result String String)
linkify string =
    List.reverse (linkifyHelp (String.words string) [])


linkifyHelp : List String -> List (Result String String) -> List (Result String String)
linkifyHelp words linkified =
    case words of
        [] ->
            linkified

        word :: restWords ->
            if isUrl word then
                case linkified of
                    (Err lastWord) :: restLinkified ->
                        -- append space to last word
                        linkifyHelp restWords (Ok word :: Err (lastWord ++ " ") :: restLinkified)

                    (Ok _) :: _ ->
                        -- insert space between two links
                        linkifyHelp restWords (Ok word :: Err " " :: linkified)

                    _ ->
                        linkifyHelp restWords (Ok word :: linkified)

            else
                case linkified of
                    (Err lastWord) :: restLinkified ->
                        -- concatenate with last word
                        linkifyHelp restWords (Err (lastWord ++ " " ++ word) :: restLinkified)

                    (Ok _) :: _ ->
                        -- insert space after the link
                        linkifyHelp restWords (Err (" " ++ word) :: linkified)

                    _ ->
                        linkifyHelp restWords (Err word :: linkified)


isUrl : String -> Bool
isUrl =
    (\b a -> String.startsWith a b) >> (\b a -> List.any a b) [ "http://", "https://" ]
