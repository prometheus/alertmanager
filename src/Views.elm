module Views exposing (..)

-- External Imports

import Html exposing (..)
import Html.Attributes exposing (..)
import Html.Events exposing (..)
import String


-- Internal Imports

import Types exposing (Model, Silence, Alert, Matcher, Msg, Route(..))


view : Model -> Html Msg
view model =
    case model.route of
        AlertsRoute ->
            genericListView todoView model.alerts

        AlertRoute name ->
            let
                one =
                    Debug.log "view: name" name
            in
                todoView model.alert

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
                , href ("#/silence/" ++ (toString silence.id))
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
