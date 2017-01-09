module Silences.Views exposing (..)

-- External Imports

import Html exposing (..)
import Html.Attributes exposing (..)


-- Internal Imports

import Types exposing (Silence)
import Utils.Views exposing (..)


silenceView : Silence -> Html msg
silenceView silence =
    let
        dictMatchers =
            List.map (\x -> ( x.name, x.value )) silence.matchers
    in
        div []
            [ dl [ class "mt2 f6 lh-copy" ]
                [ objectData (toString silence.id)
                , objectData silence.createdBy
                , objectData silence.comment
                ]
            , ul [ class "list" ]
                (List.map labelButton dictMatchers)
            , a
                [ class "f6 link br2 ba ph3 pv2 mr2 dib dark-blue"
                , href ("#/silences/" ++ (toString silence.id) ++ "/edit")
                ]
                [ text "Create" ]
            ]


objectData : String -> Html msg
objectData data =
    dt [ class "m10 black w-100" ] [ text data ]
