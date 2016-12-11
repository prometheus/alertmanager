module Views exposing (..)

-- External Imports

import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (..)
import String
import Tuple
import Utils.Date exposing (..)


-- Internal Imports

import Types exposing (Model, Silence, AlertGroup, Block, Alert, Matcher, Msg, Route(..))


view : Model -> Html Msg
view model =
    case model.route of
        AlertGroupsRoute ->
            genericListView alertGroupsView model.alertGroups

        NewSilenceRoute ->
            silenceFormView "New" model.silence

        EditSilenceRoute id ->
            silenceFormView "Edit" model.silence

        SilencesRoute ->
            genericListView silenceListView model.silences

        SilenceRoute name ->
            let
                one =
                    Debug.log "view: name" name
            in
                div []
                    [ silenceView model.silence
                    , ul [ class "list" ]
                        (List.map matcherView model.silence.matchers)
                    ]

        _ ->
            notFoundView model


todoView : a -> Html Msg
todoView model =
    div []
        [ h1 [] [ text "todo" ]
        ]


silenceFormView : String -> Silence -> Html Msg
silenceFormView kind silence =
    div [ class "pa4 black-80" ]
        [ fieldset [ class "ba b--transparent ph0 mh0" ]
            [ legend [ class "ph0 mh0 fw6" ] [ text <| kind ++ " Silence" ]
            ]
        ]


alertGroupsView : AlertGroup -> Html Msg
alertGroupsView alertGroup =
    let
        blocks =
            case alertGroup.blocks of
                Just blocks ->
                    blocks

                Nothing ->
                    []

        pairs =
            List.map2 (,) alertGroup.labels blocks
    in
        li [ class "pa3 pa4-ns bb b--black-10" ]
            [ div [] (List.map labelHeader alertGroup.labels)
            , div [] (List.map blockView blocks)
            ]


blockView : Block -> Html msg
blockView block =
    -- Block level
    div [] <|
        p [] [ text "one block" ]
            :: (List.map alertView block.alerts)


alertView : Alert -> Html msg
alertView alert =
    let
        id =
            case alert.silenceId of
                Just id ->
                    id

                Nothing ->
                    0

        b =
            if alert.silenced then
                button "Silenced" ("#/silences/" ++ toString id)
            else
                button "Active" "#/alerts"
    in
        div []
            [ p [] [ text <| Utils.Date.dateFormat alert.startsAt ]
            , b
            ]


button : String -> String -> Html msg
button txt link =
    a [ class "f6 link dim br-pill ba ph3 pv2 mb2 dib dark-blue", href link ] [ text txt ]


labelHeader : ( String, String ) -> Html msg
labelHeader ( key, value ) =
    let
        color =
            if key == "alertname" then
                "bg-red white"
            else
                ""
    in
        a [ class <| "no-underline near-white bg-animate bg-near-black hover-bg-gray inline-flex items-center ma1 tc br2 pa2 " ++ color ]
            [ text <| key ++ "=" ++ value ]


notFoundView : a -> Html Msg
notFoundView model =
    div []
        [ h1 [] [ text "not found" ]
        ]


genericListView : (a -> Html Msg) -> List a -> Html Msg
genericListView fn list =
    ul
        [ classList
            [ ( "list", True )
            , ( "pa0", True )
            ]
        ]
        (List.map fn list)


silenceListView : Silence -> Html Msg
silenceListView silence =
    let
        -- TODO: Check with fabxc if the alert being in the first position can
        -- be relied upon.
        alertName =
            case List.head silence.matchers of
                Just m ->
                    m.value

                Nothing ->
                    ""
    in
        li
            [ class "pa3 pa4-ns bb b--black-10" ]
            [ a
                [ class "db link dim blue"
                , href ("#/silences/" ++ (toString silence.id))
                ]
                [ b [ class "db f4 mb1" ]
                    [ text alertName ]
                ]
            , span [ class "f5 db lh-copy measure" ]
                [ text silence.createdBy ]
            , span [ class "f5 db lh-copy measure" ]
                [ text silence.comment ]
            ]


silenceView : Silence -> Html msg
silenceView silence =
    div
        [ classList
            [ ( "fl", True )
            , ( "w-50", False )
            , ( "pa2", True )
            , ( "ma1", True )
            , ( "w-25-m", True )
            , ( "w-w-20-l", True )
            , ( "ba b--gray", True )
            ]
        ]
        [ dl
            [ classList
                [ ( "mt2", True )
                , ( "f6", True )
                , ( "lh-copy", True )
                ]
            ]
            [ objectData (toString silence.id)
            , objectData silence.createdBy
            , objectData silence.comment
            ]
        ]


matcherView : Matcher -> Html msg
matcherView matcher =
    li [ class "dib mr1 mb2" ]
        [ a [ href "#", class "f6 b db pa2 link dim dark-gray ba b--black-20 truncate" ]
            [ text (String.join " " [ matcher.name, "=", matcher.value ]) ]
        ]


objectData : String -> Html msg
objectData data =
    dt [ class "m10 black w-100" ] [ text data ]
