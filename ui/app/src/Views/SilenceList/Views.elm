module Views.SilenceList.Views exposing (..)

-- External Imports

import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (onClick, onInput)
import Views.SilenceList.Types exposing (SilenceListMsg(..))
import Views.Shared.SilenceBase
import Silences.Types exposing (Silence)
import Utils.Types exposing (Matcher, ApiResponse(..), Filter, ApiData)
import Utils.Views exposing (iconButtonMsg, checkbox, textField, formInput, formField, buttonLink, error, loading)
import Time
import Types exposing (Msg(UpdateFilter, MsgForSilenceList, Noop))


view : ApiData (List Silence) -> ApiData Silence -> Time.Time -> Filter -> Html Msg
view apiSilences apiSilence currentTime filter =
    case apiSilences of
        Success sils ->
            -- Add buttons at the top to filter Active/Pending/Expired
            silences sils filter (text "")

        Loading ->
            loading

        Failure msg ->
            silences [] filter (error msg)


silences : List Silence -> Filter -> Html Msg -> Html Msg
silences silences filter errorHtml =
    let
        filterText =
            Maybe.withDefault "" filter.text

        html =
            if List.isEmpty silences then
                div [ class "mt2" ] [ text "no silences found" ]
            else
                ul
                    [ classList
                        [ ( "list", True )
                        , ( "pa0", True )
                        ]
                    ]
                    (List.map silenceList silences)
    in
        div []
            [ textField "Filter" filterText (UpdateFilter filter)
            , a [ class "f6 link br2 ba ph3 pv2 mr2 dib blue", onClick (MsgForSilenceList FilterSilences) ] [ text "Filter Silences" ]
            , a [ class "f6 link br2 ba ph3 pv2 mr2 dib blue", href "#/silences/new" ] [ text "New Silence" ]
            , errorHtml
            , html
            ]


silenceList : Silence -> Html Msg
silenceList silence =
    li
        [ class "pa3 pa4-ns bb b--black-10" ]
        [ Views.Shared.SilenceBase.view silence
        ]
